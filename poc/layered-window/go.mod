module github.com/deskcalendar/poc/layered-window

go 1.25.0

require (
	github.com/gogpu/gg v0.0.0-00010101000000-000000000000
	golang.org/x/sys v0.46.0
)

require (
	github.com/gogpu/gpucontext v0.21.0 // indirect
	github.com/gogpu/gputypes v0.5.1 // indirect
	golang.org/x/image v0.43.0 // indirect
	golang.org/x/text v0.39.0 // indirect
)

// Use the exact local clone (HEAD f0b4f54) we evaluated, so the gg API in
// draw_gg.go matches byte-for-byte. All of gg's deps are already in the local
// module cache (offline), so no network is needed.
replace github.com/gogpu/gg => D:/workspace/github/gg
