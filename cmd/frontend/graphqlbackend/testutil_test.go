package graphqlbackend

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/zoekt"
	"github.com/sourcegraph/sourcegraph/cmd/frontend/backend"
	"github.com/sourcegraph/sourcegraph/internal/api"
	"github.com/sourcegraph/sourcegraph/internal/database"
	"github.com/sourcegraph/sourcegraph/internal/search/result"
	"github.com/sourcegraph/sourcegraph/internal/types"
)

func resetMocks() {
	database.Mocks = database.MockStores{}
	backend.Mocks = backend.MockServices{}
}

func assertRepoResolverHydrated(ctx context.Context, t *testing.T, r *RepositoryResolver, hydrated *types.Repo) {
	t.Helper()

	description, err := r.Description(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if description != hydrated.Description {
		t.Fatalf("wrong Description. want=%q, have=%q", hydrated.Description, description)
	}

	uri, err := r.URI(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if uri != hydrated.URI {
		t.Fatalf("wrong URI. want=%q, have=%q", hydrated.URI, uri)
	}
}

func mkFileMatch(repo types.RepoName, path string, lineNumbers ...int32) *result.FileMatch {
	var lines []*result.LineMatch
	for _, n := range lineNumbers {
		lines = append(lines, &result.LineMatch{LineNumber: n})
	}
	return &result.FileMatch{
		File: result.File{
			Path: path,
			Repo: repo,
		},
		LineMatches: lines,
	}
}

func generateRepos(count int) ([]types.RepoName, []*zoekt.RepoListEntry) {
	repos := make([]types.RepoName, 0, count)
	zoektRepos := make([]*zoekt.RepoListEntry, 0, count)

	for i := 1; i <= count; i++ {
		name := fmt.Sprintf("repo-%d", i)

		repoWithIDs := types.RepoName{
			ID:   api.RepoID(i),
			Name: api.RepoName(name),
		}

		repos = append(repos, repoWithIDs)

		zoektRepos = append(zoektRepos, &zoekt.RepoListEntry{
			Repository: zoekt.Repository{
				ID:       uint32(i),
				Name:     name,
				Branches: []zoekt.RepositoryBranch{{Name: "HEAD", Version: "deadbeef"}},
			},
		})
	}
	return repos, zoektRepos
}

func generateZoektMatches(count int) []zoekt.FileMatch {
	var zoektFileMatches []zoekt.FileMatch
	for i := 1; i <= count; i++ {
		repoName := fmt.Sprintf("repo-%d", i)
		fileName := fmt.Sprintf("foobar-%d.go", i)

		zoektFileMatches = append(zoektFileMatches, zoekt.FileMatch{
			Score:        5.0,
			FileName:     fileName,
			RepositoryID: uint32(i),
			Repository:   repoName, // Important: this needs to match a name in `repos`
			Branches:     []string{"master"},
			LineMatches: []zoekt.LineMatch{
				{
					Line: nil,
				},
			},
			Checksum: []byte{0, 1, 2},
		})
	}
	return zoektFileMatches
}
