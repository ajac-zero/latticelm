package usage

import "context"

// recorderKey is the context key for the usage store.
type recorderKey struct{}

// WithRecorder adds a usage Store to the context.
func WithRecorder(ctx context.Context, store *Store) context.Context {
	return context.WithValue(ctx, recorderKey{}, store)
}

// RecordFromContext records a usage event using the Store in the context, if any.
func RecordFromContext(ctx context.Context, evt UsageEvent) {
	if store, ok := ctx.Value(recorderKey{}).(*Store); ok && store != nil {
		store.Record(evt)
	}
}
