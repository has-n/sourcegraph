package store

import (
	"context"
	"strings"
	"time"

	"github.com/keegancsmith/sqlf"
	"github.com/lib/pq"
	otlog "github.com/opentracing/opentracing-go/log"

	"github.com/sourcegraph/sourcegraph/enterprise/internal/codeintel/uploads/shared"
	"github.com/sourcegraph/sourcegraph/internal/database/basestore"
	"github.com/sourcegraph/sourcegraph/internal/database/batch"
	"github.com/sourcegraph/sourcegraph/internal/observation"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

func (s *store) InsertDefinitionsForRanking(
	ctx context.Context,
	rankingGraphKey string,
	rankingBatchNumber int,
	definitions []shared.RankingDefinitions,
) (err error) {
	ctx, _, endObservation := s.operations.insertDefinitionsForRanking.With(
		ctx,
		&err,
		observation.Args{},
	)
	defer endObservation(1, observation.Args{})

	tx, err := s.db.Transact(ctx)
	if err != nil {
		return err
	}
	defer func() { err = tx.Done(err) }()

	inserter := func(inserter *batch.Inserter) error {
		batchDefinitions := make([]shared.RankingDefinitions, 0, rankingBatchNumber)
		for _, def := range definitions {
			batchDefinitions = append(batchDefinitions, def)

			if len(batchDefinitions) == rankingBatchNumber {
				if err := insertDefinitions(ctx, inserter, rankingGraphKey, batchDefinitions); err != nil {
					return err
				}
				batchDefinitions = make([]shared.RankingDefinitions, 0, rankingBatchNumber)
			}
		}

		if len(batchDefinitions) > 0 {
			if err := insertDefinitions(ctx, inserter, rankingGraphKey, batchDefinitions); err != nil {
				return err
			}
		}

		return nil
	}

	if err := batch.WithInserter(
		ctx,
		tx.Handle(),
		"codeintel_ranking_definitions",
		batch.MaxNumPostgresParameters,
		[]string{
			"upload_id",
			"symbol_name",
			"repository",
			"document_path",
			"graph_key",
		},
		inserter,
	); err != nil {
		return err
	}

	return nil
}

func insertDefinitions(
	ctx context.Context,
	inserter *batch.Inserter,
	rankingGraphKey string,
	definitions []shared.RankingDefinitions,
) error {
	for _, def := range definitions {
		if err := inserter.Insert(
			ctx,
			def.UploadID,
			def.SymbolName,
			def.Repository,
			def.DocumentPath,
			rankingGraphKey,
		); err != nil {
			return err
		}
	}
	return nil
}

func (s *store) InsertReferencesForRanking(
	ctx context.Context,
	rankingGraphKey string,
	rankingBatchNumber int,
	references shared.RankingReferences,
) (err error) {
	ctx, _, endObservation := s.operations.insertReferencesForRanking.With(
		ctx,
		&err,
		observation.Args{},
	)
	defer endObservation(1, observation.Args{})

	tx, err := s.db.Transact(ctx)
	if err != nil {
		return err
	}
	defer func() { err = tx.Done(err) }()

	inserter := func(inserter *batch.Inserter) error {
		batchSymbolNames := make([]string, 0, rankingBatchNumber)
		for _, ref := range references.SymbolNames {
			batchSymbolNames = append(batchSymbolNames, ref)

			if len(batchSymbolNames) == rankingBatchNumber {
				if err := inserter.Insert(ctx, references.UploadID, pq.Array(batchSymbolNames), rankingGraphKey); err != nil {
					return err
				}
				batchSymbolNames = make([]string, 0, rankingBatchNumber)
			}
		}

		if len(batchSymbolNames) > 0 {
			if err := inserter.Insert(ctx, references.UploadID, pq.Array(batchSymbolNames), rankingGraphKey); err != nil {
				return err
			}
		}

		return nil
	}

	if err := batch.WithInserter(
		ctx,
		tx.Handle(),
		"codeintel_ranking_references",
		batch.MaxNumPostgresParameters,
		[]string{"upload_id", "symbol_names", "graph_key"},
		inserter,
	); err != nil {
		return err
	}

	return nil
}

func (s *store) InsertPathCountInputs(
	ctx context.Context,
	derivativeGraphKey string,
	batchSize int,
) (
	numReferenceRecordsProcessed int,
	numInputsInserted int,
	err error,
) {
	ctx, _, endObservation := s.operations.insertPathCountInputs.With(ctx, &err, observation.Args{})
	defer endObservation(1, observation.Args{})

	parts := strings.Split(derivativeGraphKey, "-")
	if len(parts) < 2 {
		return 0, 0, errors.Newf("unexpected graph key format %q", derivativeGraphKey)
	}
	// Remove last segment, which indicates the current time bucket
	parentGraphKey := strings.Join(parts[:len(parts)-1], "-")

	rows, err := s.db.Query(ctx, sqlf.Sprintf(
		insertPathCountInputsQuery,
		parentGraphKey,
		derivativeGraphKey,
		batchSize,
		derivativeGraphKey,
		derivativeGraphKey,
		parentGraphKey,
		derivativeGraphKey,
	))
	if err != nil {
		return 0, 0, err
	}
	defer func() { err = basestore.CloseRows(rows, err) }()

	for rows.Next() {
		if err := rows.Scan(
			&numReferenceRecordsProcessed,
			&numInputsInserted,
		); err != nil {
			return 0, 0, err
		}
	}

	return numReferenceRecordsProcessed, numInputsInserted, nil
}

const insertPathCountInputsQuery = `
WITH
refs AS (
	SELECT
		rr.id,
		rr.upload_id,
		rr.symbol_names
	FROM codeintel_ranking_references rr
	WHERE
		rr.graph_key = %s AND
		NOT EXISTS (
			SELECT 1
			FROM codeintel_ranking_references_processed rrp
			WHERE
				rrp.graph_key = %s AND
				rrp.codeintel_ranking_reference_id = rr.id
		)
	ORDER BY rr.id
	LIMIT %s
),
locked_refs AS (
	INSERT INTO codeintel_ranking_references_processed (graph_key, codeintel_ranking_reference_id)
	SELECT %s, r.id FROM refs r
	ON CONFLICT DO NOTHING
	RETURNING codeintel_ranking_reference_id
),
processable_symbols AS (
	SELECT r.symbol_names
	FROM locked_refs lr
	JOIN refs r ON r.id = lr.codeintel_ranking_reference_id
	JOIN lsif_uploads u ON u.id = r.upload_id
	WHERE
		-- Do not re-process references for repository/root/indexers that have already been
		-- processed. We'll still insert a processed reference so that we know we've done the
		-- "work", but we'll simply no-op the counts for this input.
		NOT EXISTS (
			SELECT 1
			FROM lsif_uploads u2
			JOIN codeintel_ranking_references rr ON rr.upload_id = u2.id
			JOIN codeintel_ranking_references_processed rrp ON rrp.codeintel_ranking_reference_id = rr.id
			WHERE
				rrp.graph_key = %s AND
				u.repository_id = u2.repository_id AND
				u.root = u2.root AND
				u.indexer = u2.indexer
		) AND
		-- For multiple references for the same repository/root/indexer in THIS batch, we want to
		-- process the one associated with the most recently processed upload record. This should
		-- maximize fresh results.
		NOT EXISTS (
			SELECT 1
			FROM locked_refs lr2
			JOIN refs r2 ON r2.id = lr2.codeintel_ranking_reference_id
			JOIN lsif_uploads u2 ON u2.id = r2.upload_id
			WHERE
				u.repository_id = u2.repository_id AND
				u.root = u2.root AND
				u.indexer = u2.indexer AND
				u.finished_at < u2.finished_at
		)
),
referenced_symbols AS (
	SELECT unnest(r.symbol_names) AS symbol_name
	FROM processable_symbols r
),
referenced_definitions AS (
	SELECT
		rd.repository,
		rd.document_path,
		rd.graph_key,
		COUNT(*) AS count
	FROM codeintel_ranking_definitions rd
	JOIN referenced_symbols rs ON rs.symbol_name = rd.symbol_name
	WHERE rd.graph_key = %s
	GROUP BY rd.repository, rd.document_path, rd.graph_key
),
ins AS (
	INSERT INTO codeintel_ranking_path_counts_inputs (repository, document_path, count, graph_key)
	SELECT
		rx.repository,
		rx.document_path,
		SUM(rx.count),
		%s
	FROM referenced_definitions rx
	GROUP BY rx.repository, rx.document_path
	RETURNING 1
)
SELECT
	(SELECT COUNT(*) FROM locked_refs),
	(SELECT COUNT(*) FROM ins)
`

func (s *store) InsertPathRanks(
	ctx context.Context,
	derivativeGraphKey string,
	batchSize int,
) (numPathRanksInserted float64, numInputsProcessed float64, err error) {
	ctx, _, endObservation := s.operations.insertPathRanks.With(
		ctx,
		&err,
		observation.Args{LogFields: []otlog.Field{
			otlog.String("derivativeGraphKey", derivativeGraphKey),
		}},
	)
	defer endObservation(1, observation.Args{})

	// Unused, but here for validation
	if parts := strings.Split(derivativeGraphKey, "-"); len(parts) < 2 {
		return 0, 0, errors.Newf("unexpected graph key format %q", derivativeGraphKey)
	}

	rows, err := s.db.Query(ctx, sqlf.Sprintf(
		insertPathRanksQuery,
		derivativeGraphKey,
		batchSize,
		derivativeGraphKey,
	))
	if err != nil {
		return 0, 0, err
	}
	defer func() { err = basestore.CloseRows(rows, err) }()

	if !rows.Next() {
		return 0, 0, errors.New("no rows from count")
	}

	if err = rows.Scan(&numPathRanksInserted, &numInputsProcessed); err != nil {
		return 0, 0, err
	}

	return numPathRanksInserted, numInputsProcessed, nil
}

const insertPathRanksQuery = `
WITH
input_ranks AS (
	SELECT
		pci.id,
		r.id AS repository_id,
		pci.document_path AS path,
		pci.count
	FROM codeintel_ranking_path_counts_inputs pci
	JOIN repo r ON lower(r.name) = (lower(pci.repository::text) COLLATE "C")
	WHERE
		pci.graph_key = %s AND
		NOT pci.processed AND
		r.deleted_at IS NULL AND
		r.blocked IS NULL
	ORDER BY pci.graph_key, pci.repository, pci.id
	LIMIT %s
	FOR UPDATE SKIP LOCKED
),
processed AS (
	UPDATE codeintel_ranking_path_counts_inputs
	SET processed = true
	WHERE id IN (SELECT ir.id FROM input_ranks ir)
	RETURNING 1
),
inserted AS (
	INSERT INTO codeintel_path_ranks AS pr (repository_id, precision, graph_key, payload)
	SELECT
		temp.repository_id,
		1,
		%s,
		sg_jsonb_concat_agg(temp.row)
	FROM (
		SELECT
			cr.repository_id,
			jsonb_build_object(cr.path, SUM(count)) AS row
		FROM input_ranks cr
		GROUP BY cr.repository_id, cr.path
	) temp
	GROUP BY temp.repository_id
	ON CONFLICT (repository_id, precision) DO UPDATE SET
		graph_key = EXCLUDED.graph_key,
		payload = CASE
			WHEN pr.graph_key != EXCLUDED.graph_key
				THEN EXCLUDED.payload
			ELSE
				(
					SELECT sg_jsonb_concat_agg(row) FROM (
						SELECT jsonb_build_object(key, SUM(value::int)) AS row
						FROM
							(
								SELECT * FROM jsonb_each(pr.payload)
								UNION
								SELECT * FROM jsonb_each(EXCLUDED.payload)
							) AS both_payloads
						GROUP BY key
					) AS combined_json
				)
			END
	RETURNING 1
)
SELECT
	(SELECT COUNT(*) FROM processed) AS num_processed,
	(SELECT COUNT(*) FROM inserted) AS num_inserted
`

// TODO - configure via envvar
const vacuumBatchSize = 1000

// TODO - configure via envvar
var threshold = time.Duration(1) * time.Hour

func (s *store) VacuumStaleDefinitionsAndReferences(ctx context.Context, graphKey string) (
	numStaleDefinitionRecordsDeleted int,
	numStaleReferenceRecordsDeleted int,
	err error,
) {
	ctx, _, endObservation := s.operations.vacuumStaleDefinitionsAndReferences.With(ctx, &err, observation.Args{LogFields: []otlog.Field{}})
	defer endObservation(1, observation.Args{})

	rows, err := s.db.Query(ctx, sqlf.Sprintf(
		vacuumStaleDefinitionsAndReferencesQuery,
		graphKey, int(threshold/time.Hour), vacuumBatchSize,
		graphKey, int(threshold/time.Hour), vacuumBatchSize,
	))
	if err != nil {
		return 0, 0, err
	}
	defer func() { err = basestore.CloseRows(rows, err) }()

	for rows.Next() {
		if err := rows.Scan(
			&numStaleDefinitionRecordsDeleted,
			&numStaleReferenceRecordsDeleted,
		); err != nil {
			return 0, 0, err
		}
	}

	return numStaleDefinitionRecordsDeleted, numStaleReferenceRecordsDeleted, nil
}

const vacuumStaleDefinitionsAndReferencesQuery = `
WITH
locked_definitions AS (
	SELECT
		rd.id,
		rd.upload_id,
		EXISTS (SELECT 1 FROM lsif_uploads_visible_at_tip uvt WHERE uvt.upload_id = rd.upload_id AND uvt.is_default_branch) AS safe
	FROM codeintel_ranking_definitions rd
	WHERE
		rd.graph_key = %s AND
		(rd.last_scanned_at IS NULL OR NOW() - rd.last_scanned_at >= %s * '1 hour'::interval)
	ORDER BY rd.last_scanned_at ASC NULLS FIRST
	FOR UPDATE SKIP LOCKED
	LIMIT %s
),
updated_definitions AS (
	UPDATE codeintel_ranking_definitions
	SET last_scanned_at = NOW()
	WHERE id IN (SELECT ld.id FROM locked_definitions ld WHERE ld.safe)
),
deleted_definitions AS (
	DELETE FROM codeintel_ranking_definitions
	WHERE id IN (SELECT ld.id FROM locked_definitions ld WHERE NOT ld.safe)
	RETURNING 1
),
locked_references AS (
	SELECT
		rr.id,
		rr.upload_id,
		EXISTS (SELECT 1 FROM lsif_uploads_visible_at_tip uvt WHERE uvt.upload_id = rr.upload_id AND uvt.is_default_branch) AS safe
	FROM codeintel_ranking_references rr
	WHERE
		rr.graph_key = %s AND
		(rr.last_scanned_at IS NULL OR NOW() - rr.last_scanned_at >= %s * '1 hour'::interval)
	ORDER BY rr.last_scanned_at ASC NULLS FIRST
	FOR UPDATE SKIP LOCKED
	LIMIT %s
),
updated_references AS (
	UPDATE codeintel_ranking_references
	SET last_scanned_at = NOW()
	WHERE id IN (SELECT lr.id FROM locked_references lr WHERE lr.safe)
),
deleted_references AS (
	DELETE FROM codeintel_ranking_references
	WHERE id IN (SELECT lr.id FROM locked_references lr WHERE NOT lr.safe)
	RETURNING 1
)
SELECT
	(SELECT COUNT(*) FROM deleted_definitions),
	(SELECT COUNT(*) FROM deleted_references)
`

func (s *store) VacuumStaleGraphs(ctx context.Context, derivativeGraphKey string) (
	metadataRecordsDeleted int,
	inputRecordsDeleted int,
	err error,
) {
	ctx, _, endObservation := s.operations.vacuumStaleGraphs.With(ctx, &err, observation.Args{LogFields: []otlog.Field{}})
	defer endObservation(1, observation.Args{})

	rows, err := s.db.Query(ctx, sqlf.Sprintf(vacuumStaleGraphsQuery, derivativeGraphKey, derivativeGraphKey))
	if err != nil {
		return 0, 0, err
	}
	defer func() { err = basestore.CloseRows(rows, err) }()

	for rows.Next() {
		if err := rows.Scan(
			&metadataRecordsDeleted,
			&inputRecordsDeleted,
		); err != nil {
			return 0, 0, err
		}
	}

	return metadataRecordsDeleted, inputRecordsDeleted, nil
}

const vacuumStaleGraphsQuery = `
WITH
locked_references_processed AS (
	SELECT id
	FROM codeintel_ranking_references_processed
	WHERE graph_key != %s
	ORDER BY id
	FOR UPDATE
),
locked_path_counts_inputs AS (
	SELECT id
	FROM codeintel_ranking_path_counts_inputs
	WHERE graph_key != %s
	ORDER BY id
	FOR UPDATE
),
deleted_references_processed AS (
	DELETE FROM codeintel_ranking_references_processed
	WHERE id IN (SELECT id FROM locked_references_processed)
	RETURNING 1
),
deleted_path_counts_inputs AS (
	DELETE FROM codeintel_ranking_path_counts_inputs
	WHERE id IN (SELECT id FROM locked_path_counts_inputs)
	RETURNING 1
)
SELECT
	(SELECT COUNT(*) FROM deleted_references_processed),
	(SELECT COUNT(*) FROM deleted_path_counts_inputs)
`
