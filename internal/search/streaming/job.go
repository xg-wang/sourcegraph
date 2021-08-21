package streaming

import "context"

// Job is an interface shared by all search backends. Calling Run on search job
// objects runs the search.
type Job interface {
	Run(context.Context, Sender) error
}
