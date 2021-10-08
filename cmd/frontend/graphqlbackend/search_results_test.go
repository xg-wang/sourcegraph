package graphqlbackend

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

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
}

func TestSearchResolver_DynamicFilters(t *testing.T) {
	db := new(dbtesting.MockDB)

	repo := types.RepoName{Name: "testRepo"}
	repoMatch := &result.RepoMatch{Name: "testRepo"}
	fileMatch := func(path string) *result.FileMatch {
		return mkFileMatch(repo, path)
	}

	rev := "develop3.0"
	fileMatchRev := fileMatch("/testFile.md")
	fileMatchRev.InputRev = &rev

	type testCase struct {
		descr                             string
		searchResults                     []result.Match
		expectedDynamicFilterStrsRegexp   map[string]int
		expectedDynamicFilterStrsGlobbing map[string]int
	}

	tests := []testCase{

		{
			descr:         "single repo match",
			searchResults: []result.Match{repoMatch},
			expectedDynamicFilterStrsRegexp: map[string]int{
				`repo:^testRepo$`: 1,
			},
			expectedDynamicFilterStrsGlobbing: map[string]int{
				`repo:testRepo`: 1,
			},
		},

		{
			descr:         "single file match without revision in query",
			searchResults: []result.Match{fileMatch("/testFile.md")},
			expectedDynamicFilterStrsRegexp: map[string]int{
				`repo:^testRepo$`: 1,
				`lang:markdown`:   1,
			},
			expectedDynamicFilterStrsGlobbing: map[string]int{
				`repo:testRepo`: 1,
				`lang:markdown`: 1,
			},
		},

		{
			descr:         "single file match with specified revision",
			searchResults: []result.Match{fileMatchRev},
			expectedDynamicFilterStrsRegexp: map[string]int{
				`repo:^testRepo$@develop3.0`: 1,
				`lang:markdown`:              1,
			},
			expectedDynamicFilterStrsGlobbing: map[string]int{
				`repo:testRepo@develop3.0`: 1,
				`lang:markdown`:            1,
			},
		},
		{
			descr:         "file match from a language with two file extensions, using first extension",
			searchResults: []result.Match{fileMatch("/testFile.ts")},
			expectedDynamicFilterStrsRegexp: map[string]int{
				`repo:^testRepo$`: 1,
				`lang:typescript`: 1,
			},
			expectedDynamicFilterStrsGlobbing: map[string]int{
				`repo:testRepo`:   1,
				`lang:typescript`: 1,
			},
		},
		{
			descr:         "file match from a language with two file extensions, using second extension",
			searchResults: []result.Match{fileMatch("/testFile.tsx")},
			expectedDynamicFilterStrsRegexp: map[string]int{
				`repo:^testRepo$`: 1,
				`lang:typescript`: 1,
			},
			expectedDynamicFilterStrsGlobbing: map[string]int{
				`repo:testRepo`:   1,
				`lang:typescript`: 1,
			},
		},
		{
			descr:         "file match which matches one of the common file filters",
			searchResults: []result.Match{fileMatch("/anything/node_modules/testFile.md")},
			expectedDynamicFilterStrsRegexp: map[string]int{
				`repo:^testRepo$`:          1,
				`-file:(^|/)node_modules/`: 1,
				`lang:markdown`:            1,
			},
			expectedDynamicFilterStrsGlobbing: map[string]int{
				`repo:testRepo`: 1,
				`-file:node_modules/** -file:**/node_modules/**`: 1,
				`lang:markdown`: 1,
			},
		},
		{
			descr:         "file match which matches one of the common file filters",
			searchResults: []result.Match{fileMatch("/node_modules/testFile.md")},
			expectedDynamicFilterStrsRegexp: map[string]int{
				`repo:^testRepo$`:          1,
				`-file:(^|/)node_modules/`: 1,
				`lang:markdown`:            1,
			},
			expectedDynamicFilterStrsGlobbing: map[string]int{
				`repo:testRepo`: 1,
				`-file:node_modules/** -file:**/node_modules/**`: 1,
				`lang:markdown`: 1,
			},
		},
		{
			descr: "file match which matches one of the common file filters",
			searchResults: []result.Match{
				fileMatch("/foo_test.go"),
				fileMatch("/foo.go"),
			},
			expectedDynamicFilterStrsRegexp: map[string]int{
				`repo:^testRepo$`:  2,
				`-file:_test\.go$`: 1,
				`lang:go`:          2,
			},
			expectedDynamicFilterStrsGlobbing: map[string]int{
				`repo:testRepo`:    2,
				`-file:**_test.go`: 1,
				`lang:go`:          2,
			},
		},

		{
			descr: "prefer rust to renderscript",
			searchResults: []result.Match{
				fileMatch("/channel.rs"),
			},
			expectedDynamicFilterStrsRegexp: map[string]int{
				`repo:^testRepo$`: 1,
				`lang:rust`:       1,
			},
			expectedDynamicFilterStrsGlobbing: map[string]int{
				`repo:testRepo`: 1,
				`lang:rust`:     1,
			},
		},

		{
			descr: "javascript filters",
			searchResults: []result.Match{
				fileMatch("/jsrender.min.js.map"),
				fileMatch("playground/react/lib/app.js.map"),
				fileMatch("assets/javascripts/bootstrap.min.js"),
			},
			expectedDynamicFilterStrsRegexp: map[string]int{
				`repo:^testRepo$`:  3,
				`-file:\.min\.js$`: 1,
				`-file:\.js\.map$`: 2,
				`lang:javascript`:  1,
			},
			expectedDynamicFilterStrsGlobbing: map[string]int{
				`repo:testRepo`:   3,
				`-file:**.min.js`: 1,
				`-file:**.js.map`: 2,
				`lang:javascript`: 1,
			},
		},

		// If there are no search results, no filters should be displayed.
		{
			descr:                             "no results",
			searchResults:                     []result.Match{},
			expectedDynamicFilterStrsRegexp:   map[string]int{},
			expectedDynamicFilterStrsGlobbing: map[string]int{},
		},
		{
			descr:         "values containing spaces are quoted",
			searchResults: []result.Match{fileMatch("/.gitignore")},
			expectedDynamicFilterStrsRegexp: map[string]int{
				`repo:^testRepo$`:    1,
				`lang:"ignore list"`: 1,
			},
			expectedDynamicFilterStrsGlobbing: map[string]int{
				`repo:testRepo`:      1,
				`lang:"ignore list"`: 1,
			},
		},
	}

	mockDecodedViewerFinalSettings = &schema.Settings{}
	defer func() { mockDecodedViewerFinalSettings = nil }()

	var expectedDynamicFilterStrs map[string]int
	for _, test := range tests {
		t.Run(test.descr, func(t *testing.T) {
			for _, globbing := range []bool{true, false} {
				mockDecodedViewerFinalSettings.SearchGlobbing = &globbing
				actualDynamicFilters := (&SearchResultsResolver{db: db, SearchResults: &SearchResults{Matches: test.searchResults}}).DynamicFilters(context.Background())
				actualDynamicFilterStrs := make(map[string]int)

				for _, filter := range actualDynamicFilters {
					actualDynamicFilterStrs[filter.Value()] = int(filter.Count())
				}

				if globbing {
					expectedDynamicFilterStrs = test.expectedDynamicFilterStrsGlobbing
				} else {
					expectedDynamicFilterStrs = test.expectedDynamicFilterStrsRegexp
				}

				if diff := cmp.Diff(expectedDynamicFilterStrs, actualDynamicFilterStrs); diff != "" {
					t.Errorf("mismatch (-want, +got):\n%s", diff)
				}
			}
		})
	}
}

func TestLonger(t *testing.T) {
	N := 2
	noise := time.Nanosecond
	for dt := time.Millisecond + noise; dt < time.Hour; dt += time.Millisecond {
		dt2 := longer(N, dt)
		if dt2 < time.Duration(N)*dt {
			t.Fatalf("longer(%v)=%v < 2*%v, want more", dt, dt2, dt)
		}
		if strings.Contains(dt2.String(), ".") {
			t.Fatalf("longer(%v).String() = %q contains an unwanted decimal point, want a nice round duration", dt, dt2)
		}
		lowest := 2 * time.Second
		if dt2 < lowest {
			t.Fatalf("longer(%v) = %v < %s, too short", dt, dt2, lowest)
		}
	}
}
