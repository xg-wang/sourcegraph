package graphqlbackend

import (
	"context"
	"fmt"
	"sort"
	"testing"

	"github.com/google/go-cmp/cmp"
	"go.uber.org/atomic"

	"github.com/sourcegraph/sourcegraph/internal/conf"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/database/dbtesting"
	"github.com/sourcegraph/sourcegraph/internal/search"
	"github.com/sourcegraph/sourcegraph/internal/search/result"
	"github.com/sourcegraph/sourcegraph/internal/search/run"
	"github.com/sourcegraph/sourcegraph/internal/search/streaming"
	"github.com/sourcegraph/sourcegraph/internal/search/symbol"
	"github.com/sourcegraph/sourcegraph/internal/search/unindexed"
	"github.com/sourcegraph/sourcegraph/internal/types"
	"github.com/sourcegraph/sourcegraph/schema"
)

var mockCount = func(_ context.Context, options database.ReposListOptions) (int, error) { return 0, nil }

func assertEqual(t *testing.T, got, want interface{}) {
	t.Helper()

	if diff := cmp.Diff(got, want); diff != "" {
		t.Fatalf("(-want +got):\n%s", diff)
	}
}

func TestSearchResults(t *testing.T) {
	db := new(dbtesting.MockDB)

	limitOffset := &database.LimitOffset{Limit: search.SearchLimits(conf.Get()).MaxRepos + 1}

	getResults := func(t *testing.T, query, version string) []string {
		r, err := (&schemaResolver{db: db}).Search(context.Background(), &SearchArgs{Query: query, Version: version})
		if err != nil {
			t.Fatal("Search:", err)
		}
		results, err := r.Results(context.Background())
		if err != nil {
			t.Fatal("Results:", err)
		}
		resultDescriptions := make([]string, len(results.Matches))
		for i, match := range results.Matches {
			// NOTE: Only supports one match per line. If we need to test other cases,
			// just remove that assumption in the following line of code.
			switch m := match.(type) {
			case *result.RepoMatch:
				resultDescriptions[i] = fmt.Sprintf("repo:%s", m.Name)
			case *result.FileMatch:
				resultDescriptions[i] = fmt.Sprintf("%s:%d", m.Path, m.LineMatches[0].LineNumber)
			default:
				t.Fatal("unexpected result type", match)
			}
		}
		// dedup results since we expect our clients to do dedupping
		if len(resultDescriptions) > 1 {
			sort.Strings(resultDescriptions)
			dedup := resultDescriptions[:1]
			for _, s := range resultDescriptions[1:] {
				if s != dedup[len(dedup)-1] {
					dedup = append(dedup, s)
				}
			}
			resultDescriptions = dedup
		}
		return resultDescriptions
	}
	testCallResults := func(t *testing.T, query, version string, want []string) {
		t.Helper()
		results := getResults(t, query, version)
		if d := cmp.Diff(want, results); d != "" {
			t.Errorf("unexpected results (-want, +got):\n%s", d)
		}
	}

	searchVersions := []string{"V1", "V2"}

	t.Run("repo: only", func(t *testing.T) {
		mockDecodedViewerFinalSettings = &schema.Settings{}
		defer func() { mockDecodedViewerFinalSettings = nil }()

		var calledReposListRepoNames bool
		database.Mocks.Repos.ListRepoNames = func(_ context.Context, op database.ReposListOptions) ([]types.RepoName, error) {
			calledReposListRepoNames = true

			// Validate that the following options are invariant
			// when calling the DB through Repos.ListRepoNames, no matter how
			// many times it is called for a single Search(...) operation.
			assertEqual(t, op.LimitOffset, limitOffset)
			assertEqual(t, op.IncludePatterns, []string{"r", "p"})

			return []types.RepoName{{ID: 1, Name: "repo"}}, nil
		}
		database.Mocks.Repos.MockGetByName(t, "repo", 1)
		database.Mocks.Repos.MockGet(t, 1)
		database.Mocks.Repos.Count = mockCount

		unindexed.MockSearchFilesInRepos = func() ([]result.Match, *streaming.Stats, error) {
			return nil, &streaming.Stats{}, nil
		}
		defer func() { unindexed.MockSearchFilesInRepos = nil }()

		for _, v := range searchVersions {
			testCallResults(t, `repo:r repo:p`, v, []string{"repo:repo"})
			if !calledReposListRepoNames {
				t.Error("!calledReposListRepoNames")
			}
		}

	})

	t.Run("multiple terms regexp", func(t *testing.T) {
		mockDecodedViewerFinalSettings = &schema.Settings{}
		defer func() { mockDecodedViewerFinalSettings = nil }()

		var calledReposListRepoNames bool
		database.Mocks.Repos.ListRepoNames = func(_ context.Context, op database.ReposListOptions) ([]types.RepoName, error) {
			calledReposListRepoNames = true

			// Validate that the following options are invariant
			// when calling the DB through Repos.List, no matter how
			// many times it is called for a single Search(...) operation.
			assertEqual(t, op.LimitOffset, limitOffset)

			return []types.RepoName{{ID: 1, Name: "repo"}}, nil
		}
		defer func() { database.Mocks = database.MockStores{} }()
		database.Mocks.Repos.MockGetByName(t, "repo", 1)
		database.Mocks.Repos.MockGet(t, 1)
		database.Mocks.Repos.Count = mockCount

		calledSearchRepositories := false
		run.MockSearchRepositories = func(args *search.TextParameters) ([]result.Match, *streaming.Stats, error) {
			calledSearchRepositories = true
			return nil, &streaming.Stats{}, nil
		}
		defer func() { run.MockSearchRepositories = nil }()

		calledSearchSymbols := false
		symbol.MockSearchSymbols = func(ctx context.Context, args *search.TextParameters, limit int) (res []result.Match, common *streaming.Stats, err error) {
			calledSearchSymbols = true
			if want := `(foo\d).*?(bar\*)`; args.PatternInfo.Pattern != want {
				t.Errorf("got %q, want %q", args.PatternInfo.Pattern, want)
			}
			// TODO return mock results here and assert that they are output as results
			return nil, nil, nil
		}
		defer func() { symbol.MockSearchSymbols = nil }()

		calledSearchFilesInRepos := atomic.NewBool(false)
		unindexed.MockSearchFilesInRepos = func() ([]result.Match, *streaming.Stats, error) {
			calledSearchFilesInRepos.Store(true)
			repo := types.RepoName{ID: 1, Name: "repo"}
			fm := mkFileMatch(repo, "dir/file", 123)
			return []result.Match{fm}, &streaming.Stats{}, nil
		}
		defer func() { unindexed.MockSearchFilesInRepos = nil }()

		testCallResults(t, `foo\d "bar*"`, "V1", []string{"dir/file:123"})
		if !calledReposListRepoNames {
			t.Error("!calledReposListRepoNames")
		}
		if !calledSearchRepositories {
			t.Error("!calledSearchRepositories")
		}
		if !calledSearchFilesInRepos.Load() {
			t.Error("!calledSearchFilesInRepos")
		}
		if calledSearchSymbols {
			t.Error("calledSearchSymbols")
		}
	})

	/*
		t.Run("multiple terms literal", func(t *testing.T) {
			mockDecodedViewerFinalSettings = &schema.Settings{}
			defer func() { mockDecodedViewerFinalSettings = nil }()

			var calledReposListRepoNames bool
			database.Mocks.Repos.ListRepoNames = func(_ context.Context, op database.ReposListOptions) ([]types.RepoName, error) {
				calledReposListRepoNames = true

				// Validate that the following options are invariant
				// when calling the DB through Repos.List, no matter how
				// many times it is called for a single Search(...) operation.
				assertEqual(t, op.LimitOffset, limitOffset)

				return []types.RepoName{{ID: 1, Name: "repo"}}, nil
			}
			defer func() { database.Mocks = database.MockStores{} }()
			database.Mocks.Repos.MockGetByName(t, "repo", 1)
			database.Mocks.Repos.MockGet(t, 1)
			database.Mocks.Repos.Count = mockCount

			calledSearchRepositories := false
			run.MockSearchRepositories = func(args *search.TextParameters) ([]result.Match, *streaming.Stats, error) {
				calledSearchRepositories = true
				return nil, &streaming.Stats{}, nil
			}
			defer func() { run.MockSearchRepositories = nil }()

			calledSearchSymbols := false
			symbol.MockSearchSymbols = func(ctx context.Context, args *search.TextParameters, limit int) (res []result.Match, common *streaming.Stats, err error) {
				calledSearchSymbols = true
				if want := `"foo\\d \"bar*\""`; args.PatternInfo.Pattern != want {
					t.Errorf("got %q, want %q", args.PatternInfo.Pattern, want)
				}
				// TODO return mock results here and assert that they are output as results
				return nil, nil, nil
			}
			defer func() { symbol.MockSearchSymbols = nil }()

			calledSearchFilesInRepos := atomic.NewBool(false)
			unindexed.MockSearchFilesInRepos = func() ([]result.Match, *streaming.Stats, error) {
				calledSearchFilesInRepos.Store(true)
				repo := types.RepoName{ID: 1, Name: "repo"}
				fm := mkFileMatch(repo, "dir/file", 123)
				return []result.Match{fm}, &streaming.Stats{}, nil
			}
			defer func() { unindexed.MockSearchFilesInRepos = nil }()

			testCallResults(t, `foo\d "bar*"`, "V2", []string{"dir/file:123"})
			if !calledReposListRepoNames {
				t.Error("!calledReposListRepoNames")
			}
			if !calledSearchRepositories {
				t.Error("!calledSearchRepositories")
			}
			if !calledSearchFilesInRepos.Load() {
				t.Error("!calledSearchFilesInRepos")
			}
			if calledSearchSymbols {
				t.Error("calledSearchSymbols")
			}
		})
	*/
}
