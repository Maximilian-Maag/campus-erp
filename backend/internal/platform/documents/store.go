package documents

import "context"

// Store keeps a rendered document under a key. It is the subset of storage.Store the
// document workflow needs, so a storage.Store can be handed straight to it.
type Store interface {
	Save(ctx context.Context, key string, content []byte) error
}
