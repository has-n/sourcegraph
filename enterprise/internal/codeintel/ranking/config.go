package ranking

import (
	"time"

	"github.com/sourcegraph/sourcegraph/internal/env"
)

type rankingConfig struct {
	env.BaseConfig

	DocumentReferenceCountsEnabled  bool
	DocumentReferenceCountsGraphKey string
	SymbolExporterInterval          time.Duration
	SymbolExporterNumRoutines       int
	SymbolExporterReadBatchSize     int
	SymbolExporterWriteBatchSize    int
	MapReducerInterval              time.Duration
	MapperBatchSize                 int
	ReducerBatchSize                int
}

var ConfigInst = &rankingConfig{}

func (c *rankingConfig) Load() {
	c.DocumentReferenceCountsEnabled = c.GetBool("CODEINTEL_RANKING_DOCUMENT_REFERENCE_COUNTS_ENABLED", "false", "Whether or not to run the ranking job.")
	c.DocumentReferenceCountsGraphKey = c.Get("CODEINTEL_RANKING_DOCUMENT_REFERENCE_COUNTS_GRAPH_KEY", "dev", "Backdoor value used to restart the ranking export procedure.")
	c.SymbolExporterInterval = c.GetInterval("CODEINTEL_RANKING_SYMBOL_EXPORTER_INTERVAL", "1s", "How frequently to serialize a batch of the code intel graph for ranking.")
	c.SymbolExporterNumRoutines = c.GetInt("CODEINTEL_RANKING_SYMBOL_EXPORTER_NUM_ROUTINES", "4", "The number of concurrent ranking graph serializer routines to run per worker instance.")
	c.SymbolExporterReadBatchSize = c.GetInt("CODEINTEL_RANKING_SYMBOL_EXPORTER_READ_BATCH_SIZE", "16", "How many uploads to process at once.")
	c.SymbolExporterWriteBatchSize = c.GetInt("CODEINTEL_RANKING_SYMBOL_EXPORTER_WRITE_BATCH_SIZE", "10000", "The number of definitions and references to populate the ranking graph per batch.")
	c.MapReducerInterval = c.GetInterval("CODEINTEL_RANKING_MAP_REDUCER_INTERVAL", "72h", "The maximum age of document reference count values used for ranking before being considered stale.")
	c.MapperBatchSize = c.GetInt("CODEINTEL_RANKING_MAPPER_BATCH_SIZE", "1000", "How many definitions and references to map at once.")
	c.ReducerBatchSize = c.GetInt("CODEINTEL_RANKING_REDUCER_BATCH_SIZE", "1000", "How many path counts to reduce at once.")
}
