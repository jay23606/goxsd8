module github.com/kud360/goxsd8

go 1.26

tool (
	github.com/kud360/goxsd8/tools/backendtestgen
	github.com/kud360/goxsd8/tools/blockgen
	github.com/kud360/goxsd8/tools/casejoin
	github.com/kud360/goxsd8/tools/commentwrap
	github.com/kud360/goxsd8/tools/fetchspecs
	github.com/kud360/goxsd8/tools/gapaudit
	github.com/kud360/goxsd8/tools/hfnextract
	github.com/kud360/goxsd8/tools/issuefeed
	github.com/kud360/goxsd8/tools/landcheck
	github.com/kud360/goxsd8/tools/lanepartition
	github.com/kud360/goxsd8/tools/lanestatus
	github.com/kud360/goxsd8/tools/lint
	github.com/kud360/goxsd8/tools/opmapgen
	github.com/kud360/goxsd8/tools/rulecat
	github.com/kud360/goxsd8/tools/spec2md
	github.com/kud360/goxsd8/tools/suiteindex
	github.com/kud360/goxsd8/tools/surface
	github.com/kud360/goxsd8/tools/typespecgen
	github.com/kud360/goxsd8/tools/wipsurvey
)

require golang.org/x/net v0.56.0
