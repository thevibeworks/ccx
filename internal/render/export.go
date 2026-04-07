package render

import (
	"fmt"
	"strings"

	"github.com/thevibeworks/ccx/internal/parser"
)

type ExportOptions struct {
	Format          string
	Theme           string
	IncludeThinking bool
	IncludeAgents   bool
	Brief           bool
	TemplatePath    string
}

func Export(session *parser.Session, opts ExportOptions) (string, error) {
	if opts.Brief {
		session = BriefSession(session)
	}
	switch strings.ToLower(opts.Format) {
	case "html":
		return exportHTML(session, opts)
	case "md", "markdown":
		return exportMarkdown(session, opts)
	case "org":
		return exportOrg(session, opts)
	default:
		return "", fmt.Errorf("unsupported format: %s", opts.Format)
	}
}
