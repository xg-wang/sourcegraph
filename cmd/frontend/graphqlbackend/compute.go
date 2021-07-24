package graphqlbackend

import "context"

type ComputeArgs struct {
	Query string
}

type ComputeImplementer interface {
	Kind(context.Context) (*string, error)
	Value(context.Context) (string, error)
}

type computeResolver struct{}

func (r *schemaResolver) Compute(ctx context.Context, args *ComputeArgs) ComputeImplementer {
	return computeResolver{}
}

func (r computeResolver) Kind(ctx context.Context) (*string, error) {
	v := "kind"
	return &v, nil
}

func (r computeResolver) Value(ctx context.Context) (string, error) {
	v := "value"
	return v, nil
}
