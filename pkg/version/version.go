package version

import (
	"io"
	"runtime"
	"text/template"
)

var (
	Version   = "dev"
	BuildDate = "unknown"
)

var versionTemplate = `Version:      {{.Version}}
Go version:   {{.GoVersion}}
Built:        {{.BuildDate}}
OS/Arch:      {{.OS}}/{{.Arch}}`

// Print writes the version information to the given writer.
func Print(wr io.Writer) error {
	tmpl, err := template.New("").Parse(versionTemplate)
	if err != nil {
		return fmt.Errorf("parsing version template: %w", err)
	}

	v := struct {
		Version   string
		GoVersion string
		BuildDate string
		OS        string
		Arch      string
	}{
		Version:   Version,
		GoVersion: runtime.Version(),
		BuildDate: BuildDate,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
	return tmpl.Execute(wr, v)
}
