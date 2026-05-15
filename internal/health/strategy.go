package health

import "context"

type CheckStrategy interface {
	Check(ctx context.Context, addr string) error
}