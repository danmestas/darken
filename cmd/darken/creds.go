package main

func runCreds(args []string) error {
	if len(args) == 0 {
		args = []string{"all"}
	}
	return runSubstrateScript("scripts/stage-creds.sh", args)
}
