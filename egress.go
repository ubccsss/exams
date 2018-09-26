package main

import (
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/d4l3k/docconv"
	"github.com/ubccsss/exams/examdb"
	"github.com/urfave/cli"
)

func setupEgressCommands() cli.Command {
	return cli.Command{
		Name:    "egress",
		Aliases: []string{"e"},
		Subcommands: []cli.Command{
			{
				Name:   "ml",
				Usage:  "export data into a format that tensorflow can read",
				Action: exportToML,
			},
		},
	}
}

func exportToML(c *cli.Context) error {
	// Map each label to be 1-n, 0 represents invalid label.
	isLabel := map[string]int{}
	for i, l := range examdb.ExamLabels {
		isLabel[l] = i + 1
	}

	for _, f := range db.ProcessedFiles() {
		class, ok := isLabel[f.Name]
		if !ok {
			continue
		}

		if err := convertAndSaveFileAsML(f, class); err != nil {
			return err
		}
	}

	return nil
}

func convertAndSaveFileAsML(f *examdb.File, class int) error {
	log.Printf("Egressing %s", f)
	in, err := f.Reader()
	if err != nil {
		return err
	}
	defer in.Close()
	txt, meta, err := docconv.ConvertPDF(in)
	if err != nil {
		return err
	}

	out, err := os.OpenFile(fmt.Sprintf("ml/data/%s-%d.txt", f.Hash, class), os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer out.Close()
	if _, err := out.WriteString(f.Source); err != nil {
		return err
	}
	if _, err := out.WriteString("\n"); err != nil {
		return err
	}
	var metalines []string
	for k, v := range meta {
		metalines = append(metalines, fmt.Sprintf("%s: %s", k, v))
	}
	sort.Strings(metalines)
	for _, line := range metalines {
		if _, err := out.WriteString(line); err != nil {
			return err
		}
		if _, err := out.WriteString("\n"); err != nil {
			return err
		}
	}
	if _, err := out.WriteString(txt); err != nil {
		return err
	}

	return nil
}
