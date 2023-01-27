package codeownership

import (
	"context"
	"fmt"
	"sync"

	otlog "github.com/opentracing/opentracing-go/log"

	"github.com/sourcegraph/sourcegraph/internal/gitserver"
	codeownerspb "github.com/sourcegraph/sourcegraph/internal/own/codeowners/proto"
	"github.com/sourcegraph/sourcegraph/internal/search"
	"github.com/sourcegraph/sourcegraph/internal/search/job"
	"github.com/sourcegraph/sourcegraph/internal/search/result"
	"github.com/sourcegraph/sourcegraph/internal/search/streaming"
	"github.com/sourcegraph/sourcegraph/internal/trace"
	"github.com/sourcegraph/sourcegraph/lib/errors"
)

func NewSelectOwnersSearch(child job.Job, owning string) job.Job {
	return &selectOwnersJob{
		child:  child,
		owning: owning,
	}
}

type selectOwnersJob struct {
	child job.Job

	owning string
}

func (s *selectOwnersJob) Run(ctx context.Context, clients job.RuntimeClients, stream streaming.Sender) (alert *search.Alert, err error) {
	_, ctx, stream, finish := job.StartSpan(ctx, stream, s)
	defer finish(alert, err)

	var (
		mu   sync.Mutex
		errs error
	)

	rules := NewRulesCache()

	filteredStream := streaming.StreamFunc(func(event streaming.SearchEvent) {
		event.Results, err = getCodeOwnersFromMatches(ctx, clients.Gitserver, &rules, event.Results)
		fmt.Println("event is sent")
		if err != nil {
			mu.Lock()
			errs = errors.Append(errs, err)
			mu.Unlock()
		}
		stream.Send(event)
	})

	alert, err = s.child.Run(ctx, clients, filteredStream)
	if err != nil {
		errs = errors.Append(errs, err)
	}
	return alert, errs
}

func (s *selectOwnersJob) Name() string {
	return "SelectOwnersSearchJob"
}

func (s *selectOwnersJob) Fields(v job.Verbosity) (res []otlog.Field) {
	switch v {
	case job.VerbosityMax:
		fallthrough
	case job.VerbosityBasic:
		res = append(res,
			trace.Strings("owning", []string{s.owning}),
		)
	}
	return res
}

func (s *selectOwnersJob) Children() []job.Describer {
	return []job.Describer{s.child}
}

func (s *selectOwnersJob) MapChildren(fn job.MapFunc) job.Job {
	cp := *s
	cp.child = job.Map(s.child, fn)
	return &cp
}

func getCodeOwnersFromMatches(
	ctx context.Context,
	gitserver gitserver.Client,
	rules *RulesCache,
	matches []result.Match,
) ([]result.Match, error) {
	var errs error
	var ownerMatches []result.Match

matchesLoop:
	for _, m := range matches {
		mm, ok := m.(*result.FileMatch)
		if !ok {
			continue
		}
		file, err := rules.GetFromCacheOrFetch(ctx, gitserver, mm.Repo.Name, mm.CommitID)
		if err != nil {
			errs = errors.Append(errs, err)
			continue matchesLoop
		}
		owners := file.FindOwners(mm.File.Path)
		ownerMatches := fromProtoOwners(owners)
		fmt.Println(ownerMatches)
	}
	return ownerMatches, errs
}

func fromProtoOwners(owners []*codeownerspb.Owner) []result.OwnerMatch {
	matches := make([]result.OwnerMatch, 0, len(owners))
	for _, o := range owners {
		matches = append(matches, result.OwnerMatch{
			Email:  o.Email,
			Handle: o.Handle,
		})
	}
	return matches
}
