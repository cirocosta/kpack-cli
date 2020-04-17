package commands

import (
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
)

type TableWriter struct {
	writer *tabwriter.Writer
}

func NewTableWriter(out io.Writer, headers ...string) (*TableWriter, error) {
	writer := tabwriter.NewWriter(out, 0, 4, 4, ' ', 0)

	_, err := fmt.Fprintln(writer, strings.ToUpper(strings.Join(headers, "\t")))
	if err != nil {
		return nil, err
	}

	return &TableWriter{writer: writer}, nil
}

func (w *TableWriter) AddRow(columns ...string) error {
	_, err := fmt.Fprintln(w.writer, strings.Join(columns, "\t"))
	return err
}

func (w *TableWriter) Write() error {
	return w.writer.Flush()
}