package cli

// Standard exit codes used throughout the CLI.
//
// These follow common Unix conventions:
//   - ExitSuccess  — successful execution
//   - ExitError    — general runtime error
//   - ExitUsage    — bad command-line usage (mirrors sysexits.h EX_USAGE)
//   - ExitInterrupt — process terminated by SIGINT (128 + signal 2)
const (
	ExitSuccess   = 0
	ExitError     = 1
	ExitUsage     = 2
	ExitInterrupt = 130
)
