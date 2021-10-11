package graphqlbackend

import (
	"context"
	"reflect"
	"testing"

	"github.com/sourcegraph/sourcegraph/internal/database/dbtesting"
	"github.com/sourcegraph/sourcegraph/internal/search"
	"github.com/sourcegraph/sourcegraph/internal/search/result"
	"github.com/sourcegraph/sourcegraph/internal/search/streaming"
	"github.com/sourcegraph/sourcegraph/internal/search/symbol"
	"github.com/sourcegraph/sourcegraph/schema"
)

func TestSearchSuggestions(t *testing.T) {
	db := new(dbtesting.MockDB)

	getSuggestions := func(t *testing.T, query, version string) []string {
		t.Helper()
		r, err := (&schemaResolver{db: db}).Search(context.Background(), &SearchArgs{Query: query, Version: version})
		if err != nil {
			t.Fatal("Search:", err)
		}
		results, err := r.Suggestions(context.Background(), &searchSuggestionsArgs{})
		if err != nil {
			t.Fatal("Suggestions:", err)
		}
		resultDescriptions := make([]string, len(results))
		for i, result := range results {
			resultDescriptions[i] = testStringResult(result)
		}
		return resultDescriptions
	}
	testSuggestions := func(t *testing.T, query, version string, want []string) {
		t.Helper()
		got := getSuggestions(t, query, version)
		if !reflect.DeepEqual(got, want) {
			t.Errorf("got != want\ngot:  %v\nwant: %v", got, want)
		}
	}

	symbol.MockSearchSymbols = func(ctx context.Context, args *search.TextParameters, limit int) (res []result.Match, common *streaming.Stats, err error) {
		// TODO test symbol suggestions
		return nil, nil, nil
	}
	defer func() { symbol.MockSearchSymbols = nil }()

	mockDecodedViewerFinalSettings = &schema.Settings{}
	defer func() { mockDecodedViewerFinalSettings = nil }()

	searchVersions := []string{"V1", "V2"}

	t.Run("empty", func(t *testing.T) {
		for _, v := range searchVersions {
			testSuggestions(t, "", v, []string{})
		}
	})

	t.Run("whitespace", func(t *testing.T) {
		for _, v := range searchVersions {
			testSuggestions(t, " ", v, []string{})
		}
	})

	t.Run("no suggestions for predicate syntax", func(t *testing.T) {
		for _, v := range searchVersions {
			testSuggestions(t, "repo:contains(file:foo)", v, []string{})
		}
	})

	t.Run("no suggestions for search expressions", func(t *testing.T) {
		for _, v := range searchVersions {
			testSuggestions(t, "file:foo or file:bar", v, []string{})
		}
	})
}
