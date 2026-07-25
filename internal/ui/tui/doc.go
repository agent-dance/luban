package tui

// This file serves as a .gsx compilation test.
// Run: tui generate ./tui/
// to verify the go-tui toolchain works correctly.
//
// For Phase 0, all components are written in pure Go (without .gsx)
// to establish the architecture. .gsx templates will be introduced in
// Phase 1+ as the component tree grows more complex.
//
// Pure Go components work identically to .gsx-generated code since
// .gsx just compiles to Go struct + Render method.
