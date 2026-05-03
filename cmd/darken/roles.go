package main

// canonicalRoles is the authoritative list of the 14 darken harness
// role names in alphabetical order. It is the single source of truth
// consumed by template uploads, doctor checks, and skill rewrites.
var canonicalRoles = []string{
	"admin",
	"base",
	"darwin",
	"designer",
	"orchestrator",
	"planner-t1",
	"planner-t2",
	"planner-t3",
	"planner-t4",
	"researcher",
	"reviewer",
	"sme",
	"tdd-implementer",
	"verifier",
}
