package reconciler

import (
	"code.uber.internal/pkg/reconciler/eviction"
	"code.uber.internal/pkg/reconciler/interceptor"
	"code.uber.internal/pkg/reconciler/status"
	"go.uber.org/fx"
)

var Module = fx.Options(
	fx.Provide(
		eviction.New,
		interceptor.New,
		status.New,
		New,
	),
)
