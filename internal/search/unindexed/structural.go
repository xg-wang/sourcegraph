package unindexed

import (
	"context"

	"github.com/cockroachdb/errors"
	"github.com/inconshreveable/log15"
	"github.com/sourcegraph/sourcegraph/internal/search"
	"github.com/sourcegraph/sourcegraph/internal/search/result"
	"github.com/sourcegraph/sourcegraph/internal/search/streaming"
	zoektutil "github.com/sourcegraph/sourcegraph/internal/search/zoekt"
	"golang.org/x/sync/errgroup"
)

type repoData interface {
	repoData()
}

func (IndexedMap) repoData()    {}
func (UnindexedList) repoData() {}

type IndexedMap map[string]*search.RepositoryRevisions
type UnindexedList []*search.RepositoryRevisions

// The following type definitions compose separable concerns for running structural search.

// searchJob is a function that may run in its own Go routine.
type searchJob func() error

// withContext parameterizes a searchJob by context, enabling easy parameterizing of ctx for multiple jobs part of an errgroup.
type withContext func(context.Context) searchJob

// structuralSearchJob creates a composable function for running structural
// search. It unrollos the repoData parameters so that a job may run over either
// indexed or unindexed repos in its own Go routine. It unrolls the context
// parameter so that multiple jobs can be parameterized by the errgroup context.
func structuralSearchJob(args *search.TextParameters, stream streaming.Sender, repoData repoData) withContext {
	return func(ctx context.Context) searchJob {
		return func() error {
			switch repos := repoData.(type) {
			case IndexedMap:
				reposList := make([]*search.RepositoryRevisions, 0, len(repos))
				for _, repo := range repos {
					reposList = append(reposList, repo)
				}
				return callSearcherOverRepos(ctx, args, stream, reposList, true)
			case UnindexedList:
				return callSearcherOverRepos(ctx, args, stream, repos, false)
			}
			panic("unreachable")
		}
	}
}

func runJobs(ctx context.Context, jobs []withContext) (err error) {
	g, ctx := errgroup.WithContext(ctx)
	for _, job := range jobs {
		g.Go(job(ctx))
	}

	return g.Wait()
}

func repoSets(indexed *zoektutil.IndexedSearchRequest, mode search.GlobalSearchMode) []repoData {
	repoSets := []repoData{UnindexedList(indexed.Unindexed)}
	if mode != search.SearcherOnly {
		repoSets = append(repoSets, IndexedMap(indexed.Repos()))
	}
	return repoSets
}

func streamStructuralSearch(ctx context.Context, args *search.TextParameters, stream streaming.Sender) error {
	ctx, stream, cleanup := streaming.WithLimit(ctx, stream, int(args.PatternInfo.FileMatchLimit))
	defer cleanup()
	indexed, err := textSearchRequest(ctx, args, zoektutil.MissingRepoRevStatus(stream))
	if err != nil {
		return err
	}

	jobs := []withContext{}
	for _, repoSet := range repoSets(indexed, args.Mode) {
		jobs = append(jobs, structuralSearchJob(args, stream, repoSet))
	}
	return runJobs(ctx, jobs)
}

func retryStructuralSearch(ctx context.Context, args *search.TextParameters, stream streaming.Sender) error {
	// For structural search with default limits we retry if we get
	// no results by forcing Zoekt to resolve more potential file
	// matches using a higher FileMatchLimit.
	patternCopy := *(args.PatternInfo)
	patternCopy.FileMatchLimit = 1000
	argsCopy := *args
	argsCopy.PatternInfo = &patternCopy
	args = &argsCopy
	return streamStructuralSearch(ctx, args, stream)

}

func StructuralSearch(ctx context.Context, args *search.TextParameters, stream streaming.Sender) error {
	if args.PatternInfo.FileMatchLimit != search.DefaultMaxSearchResults {
		// streamStructuralSearch performs a streaming search when the user sets a value
		// for `count`. The first return parameter indicates whether the request was
		// serviced with streaming.
		return streamStructuralSearch(ctx, args, stream)
	}

	fileMatches, stats, err := streaming.CollectStream(func(stream streaming.Sender) error {
		return streamStructuralSearch(ctx, args, stream)
	})
	if err != nil {
		return err
	}

	if len(fileMatches) == 0 {
		fileMatches, stats, err = streaming.CollectStream(func(stream streaming.Sender) error {
			return retryStructuralSearch(ctx, args, stream)
		})
		if err != nil {
			return err
		}
		if len(fileMatches) == 0 {
			// Still no results? Give up.
			log15.Warn("Structural search gives up after more exhaustive attempt. Results may have been missed.")
			stats.IsLimitHit = false // Ensure we don't display "Show more".
		}
	}

	matches := make([]result.Match, 0, len(fileMatches))
	for _, fm := range fileMatches {
		if _, ok := fm.(*result.FileMatch); !ok {
			return errors.Errorf("StructuralSearch failed to convert results")
		}
		matches = append(matches, fm)
	}

	stream.Send(streaming.SearchEvent{
		Results: matches,
		Stats:   stats,
	})
	return err
}
