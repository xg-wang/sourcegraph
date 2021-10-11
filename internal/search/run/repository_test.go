package run

import (
	"context"
	"crypto/md5"
	"encoding/binary"
	"reflect"
	"regexp"
	"strconv"
	"testing"

	"github.com/google/zoekt"

	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/search"
	"github.com/sourcegraph/sourcegraph/internal/search/query"
	"github.com/sourcegraph/sourcegraph/internal/search/result"
	"github.com/sourcegraph/sourcegraph/internal/search/streaming"
	"github.com/sourcegraph/sourcegraph/internal/types"
)

func searchRepositoriesBatch(ctx context.Context, args *search.TextParameters, limit int32) ([]result.Match, streaming.Stats, error) {
	return streaming.CollectStream(func(stream streaming.Sender) error {
		return SearchRepositories(ctx, args, limit, stream)
	})
}

// repoShouldBeAdded determines whether a repository should be included in the result set based on whether the repository fits in the subset
// of repostiories specified in the query's `repohasfile` and `-repohasfile` fields if they exist.
func repoShouldBeAdded(ctx context.Context, zoekt zoekt.Streamer, repo *search.RepositoryRevisions, pattern *search.TextPatternInfo) (bool, error) {
	repos := []*search.RepositoryRevisions{repo}
	args := search.TextParameters{
		PatternInfo: pattern,
		Zoekt:       zoekt,
	}
	rsta, err := reposToAdd(ctx, &args, repos)
	if err != nil {
		return false, err
	}
	return len(rsta) == 1, nil
}

func TestMatchRepos(t *testing.T) {
	want := makeRepositoryRevisions("foo/bar", "abc/foo")
	in := append(want, makeRepositoryRevisions("beef/bam", "qux/bas")...)
	pattern := regexp.MustCompile("foo")

	results := make(chan []*search.RepositoryRevisions)
	go func() {
		defer close(results)
		matchRepos(pattern, in, results)
	}()
	var repos []*search.RepositoryRevisions
	for matched := range results {
		repos = append(repos, matched...)
	}

	// because of the concurrency we cannot rely on the order of "repos" to be the
	// same as "want". Hence we create map of repo names and compare those.
	toMap := func(reporevs []*search.RepositoryRevisions) map[string]struct{} {
		out := map[string]struct{}{}
		for _, r := range reporevs {
			out[string(r.Repo.Name)] = struct{}{}
		}
		return out
	}
	if !reflect.DeepEqual(toMap(repos), toMap(want)) {
		t.Fatalf("expected %v, got %v", want, repos)
	}
}

func BenchmarkSearchRepositories(b *testing.B) {
	n := 200 * 1000
	repos := make([]*search.RepositoryRevisions, n)
	for i := 0; i < n; i++ {
		repo := types.RepoName{Name: api.RepoName("github.com/org/repo" + strconv.Itoa(i))}
		repos[i] = &search.RepositoryRevisions{Repo: repo, Revs: []search.RevisionSpecifier{{}}}
	}
	q, _ := query.ParseLiteral("context.WithValue")
	bq, _ := query.ToBasicQuery(q)
	pattern := search.ToTextPatternInfo(bq, search.Batch, query.Identity)
	tp := search.TextParameters{
		PatternInfo: pattern,
		Repos:       repos,
		Query:       q,
	}
	for i := 0; i < b.N; i++ {
		_, _, err := searchRepositoriesBatch(context.Background(), &tp, tp.PatternInfo.FileMatchLimit)
		if err != nil {
			b.Fatal(err)
		}
	}
}

func makeRepositoryRevisions(repos ...string) []*search.RepositoryRevisions {
	r := make([]*search.RepositoryRevisions, len(repos))
	for i, repospec := range repos {
		repoName, revs := search.ParseRepositoryRevisions(repospec)
		if len(revs) == 0 {
			// treat empty list as preferring master
			revs = []search.RevisionSpecifier{{RevSpec: ""}}
		}
		r[i] = &search.RepositoryRevisions{Repo: mkRepos(repoName)[0], Revs: revs}
	}
	return r
}

func mkRepos(names ...string) []types.RepoName {
	var repos []types.RepoName
	for _, name := range names {
		sum := md5.Sum([]byte(name))
		id := api.RepoID(binary.BigEndian.Uint64(sum[:]))
		if id < 0 {
			id = -(id / 2)
		}
		if id == 0 {
			id++
		}
		repos = append(repos, types.RepoName{ID: id, Name: api.RepoName(name)})
	}
	return repos
}
