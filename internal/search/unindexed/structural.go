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

// mint function that returns getRepos from text args? maybe this isn't the worst idea,
// because i can use it during query construction...

// A RepoQuery is a function that returns RepoData, and parameterized by a
// context for cancellation.
type RepoQuery func(context.Context) (zoektutil.IndexedSearchRequest, error)

// RepoData represents an object of repository revisions to search.
type RepoData interface {
	AsList() []*search.RepositoryRevisions
	IsIndexed() bool
}

type IndexedMap map[string]*search.RepositoryRevisions

func (m IndexedMap) AsList() []*search.RepositoryRevisions {
	reposList := make([]*search.RepositoryRevisions, 0, len(m))
	for _, repo := range m {
		reposList = append(reposList, repo)
	}
	return reposList
}

func (IndexedMap) IsIndexed() bool {
	return true
}

type UnindexedList []*search.RepositoryRevisions

func (ul UnindexedList) AsList() []*search.RepositoryRevisions {
	return ul
}

func (UnindexedList) IsIndexed() bool {
	return false
}

// searchRepos represent the arguments to a search called over repositories.
type searchRepos struct {
	args    *search.SearcherParameters
	repoSet RepoData
	stream  streaming.Sender
}

// getJob returns a function parameterized by ctx to search over repos.
func (s *searchRepos) getJob(ctx context.Context) func() error {
	return func() error {
		return callSearcherOverRepos(ctx, s.args, s.stream, s.repoSet.AsList(), s.repoSet.IsIndexed())
	}
}

func runJobs(ctx context.Context, jobs []*searchRepos) error {
	g, ctx := errgroup.WithContext(ctx)
	for _, j := range jobs {
		g.Go(j.getJob(ctx))
	}
	return g.Wait()
}

// repoSets returns the set of repositories to search (whether indexed or unindexed) based on search mode.
func repoSets(request zoektutil.IndexedSearchRequest, mode search.GlobalSearchMode) []RepoData {
	repoSets := []RepoData{UnindexedList(request.UnindexedRepos())} // unindexed included by default
	if mode != search.SearcherOnly {
		repoSets = append(repoSets, IndexedMap(request.IndexedRepos()))
	}
	return repoSets
}

// streamStructuralSearch runs structural search jobs and streams the results.
func streamStructuralSearch(ctx context.Context, args *search.SearcherParameters, getRepos RepoQuery, fileMatchLimit int32, mode search.GlobalSearchMode, stream streaming.Sender) (err error) {
	ctx, stream, cleanup := streaming.WithLimit(ctx, stream, int(fileMatchLimit))
	defer cleanup()

	repos, err := getRepos(ctx)
	if err != nil {
		return err
	}

	jobs := []*searchRepos{}
	for _, repoSet := range repoSets(repos, mode) {
		jobs = append(jobs, &searchRepos{args: args, stream: stream, repoSet: repoSet})
	}
	return runJobs(ctx, jobs)
}

// retryStructuralSearch runs a structural search with an updated file match limit so
// that Zoekt resolves more potential file matches.
func retryStructuralSearch(ctx context.Context, args *search.SearcherParameters, getRepos RepoQuery, fileMatchLimit int32, mode search.GlobalSearchMode, stream streaming.Sender) error {
	patternCopy := *(args.PatternInfo)
	patternCopy.FileMatchLimit = fileMatchLimit
	argsCopy := *args
	argsCopy.PatternInfo = &patternCopy
	args = &argsCopy
	return streamStructuralSearch(ctx, args, getRepos, fileMatchLimit, mode, stream)
}

func StructuralSearch(ctx context.Context, args *search.SearcherParameters, getRepos RepoQuery, fileMatchLimit int32, mode search.GlobalSearchMode, stream streaming.Sender) error {
	if fileMatchLimit != search.DefaultMaxSearchResults {
		// streamStructuralSearch performs a streaming search when the user sets a value
		// for `count`. The first return parameter indicates whether the request was
		// serviced with streaming.
		return streamStructuralSearch(ctx, args, getRepos, fileMatchLimit, mode, stream)
	}

	// For structural search with default limits we retry if we get no results.
	fileMatches, stats, err := streaming.CollectStream(func(stream streaming.Sender) error {
		return streamStructuralSearch(ctx, args, getRepos, fileMatchLimit, mode, stream)
	})

	if len(fileMatches) == 0 && err == nil {
		// retry structural search with a higher limit.
		fileMatches, stats, err = streaming.CollectStream(func(stream streaming.Sender) error {
			return retryStructuralSearch(ctx, args, getRepos, 1000, mode, stream)
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
