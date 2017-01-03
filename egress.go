package main

import (
	"fmt"
	"log"
	"os"
	"sort"

	"github.com/sajari/docconv"
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

var labels = []string{
	"Final",
	"Final (Solution)",
	"Sample Final",
	"Sample Final (Solution)",
	"Midterm",
	"Midterm (Solution)",
	"Sample Midterm",
	"Sample Midterm (Solution)",
	"Midterm 1",
	"Midterm 1 (Solution)",
	"Sample Midterm 1",
	"Sample Midterm 1 (Solution)",
	"Midterm 2",
	"Midterm 2 (Solution)",
	"Sample Midterm 2",
	"Sample Midterm 2 (Solution)",
}

func exportToML(c *cli.Context) error {
	// Map each label to be 1-n, 0 represents invalid label.
	isLabel := map[string]int{}
	for i, l := range labels {
		isLabel[l] = i + 1
	}

	for _, c := range db.Courses {
		for _, y := range c.Years {
			for _, f := range y.Files {
				class, ok := isLabel[f.Name]
				if !ok {
					continue
				}

				if err := convertAndSaveFileAsML(f, class); err != nil {
					return err
				}
			}
		}
	}

	for _, f := range db.PotentialFiles {
		if !f.NotAnExam {
			continue
		}

		if err := convertAndSaveFileAsML(f, 0); err != nil {
			return err
		}
	}

	return nil
}

func convertAndSaveFileAsML(f *File, class int) error {
	log.Printf("Egressing %s", f)
	in, err := f.reader()
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
