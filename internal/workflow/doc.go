// Package workflow owns versioned workflow definitions and graph revisions.
// It may import internal/domain and internal/events only. Durable Run and
// step execution belong to aijobrelay and stay behind owner gate D-40; this
// package must not create a second scheduler or independent Run state.
package workflow
