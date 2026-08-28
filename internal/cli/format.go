package cli

import (
	"fmt"

	"github.com/mfenderov/mark42/internal/storage"
)

// --- Helpers ---

func printEntity(e *storage.Entity) {
	output(entityStyle.Render(e.Name) + " " + typeStyle.Render("("+e.Type+")"))
	if len(e.Observations) > 0 {
		for _, obs := range e.Observations {
			output("  " + dimStyle.Render("•") + " " + obsStyle.Render(obs))
		}
	}
}

func itoa(i int) string {
	return fmt.Sprintf("%d", i)
}
