package commands

func checkMCPServers(cwd string) checkResult {
	return checkMCPServersWithBackend(cwd, nil)
}
