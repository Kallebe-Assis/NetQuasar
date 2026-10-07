package integrationhubsoft

import "context"

// PageProgressFunc recebe o avanço de uma varredura paginada: páginas já lidas e total estimado de páginas.
type PageProgressFunc func(donePages, totalPages int)

type pageProgressKey struct{}

// WithPageProgress devolve um contexto cujas varreduras paginadas (fetchAllPages) informam o avanço a fn.
func WithPageProgress(ctx context.Context, fn PageProgressFunc) context.Context {
	return context.WithValue(ctx, pageProgressKey{}, fn)
}

func reportPageProgress(ctx context.Context, done, total int) {
	if fn, ok := ctx.Value(pageProgressKey{}).(PageProgressFunc); ok && fn != nil {
		fn(done, total)
	}
}
