// Package app contains Core use cases and orchestration. It depends on
// internal/domain, the module packages (knowledgepack, aijobrelay,
// contextengine, workflow), internal/events, and internal/security ports.
// It MUST NOT import adapter implementations (storage, providers, plugins,
// platform), database drivers, HTTP frameworks, or OS-specific APIs; every
// protected use case consults the AuthorizationPolicy port before effects.
package app
