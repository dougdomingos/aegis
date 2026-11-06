package nftables

// SetRunFuncForTest replaces the executor's command runner for external tests.
func SetRunFuncForTest(executor *Executor, run func(binary string, args []string, stdin string) error) {
	executor.run = run
}
