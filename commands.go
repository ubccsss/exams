package main

import "github.com/urfave/cli"

func setupCommands() *cli.App {
	app := cli.NewApp()

	app.HelpName = "The UBCCSSS Exam App"

	app.Commands = []cli.Command{
		{
			Name:    "serve",
			Aliases: []string{"s"},
			Usage:   "serve the site on :8080",
			Action:  serveSite,
			Flags: []cli.Flag{
				cli.IntFlag{
					Name:  "port",
					Value: 8080,
					Usage: "The port to run the webserver on.",
				},
				cli.StringFlag{
					Name:  "user",
					Value: "admin",
					Usage: "Username for /admin/.",
				},
				cli.StringFlag{
					Name:  "pass",
					Value: "",
					Usage: "Password for /admin/. Admin interface is disabled if not set.",
				},
			},
		},
		{
			Name:   "indexugrad",
			Usage:  "saves all top level HTML files to archive.org",
			Action: indexUGrad,
			Flags: []cli.Flag{
				cli.StringFlag{
					Name:  "user",
					Value: "q7w9a",
					Usage: "ssh username",
				},
				cli.StringFlag{
					Name:  "server",
					Value: "annacis.ugrad.cs.ubc.ca",
					Usage: "ssh server",
				},
			},
		},
		setupEgressCommands(),
		setupIngressCommands(),
	}

	return app
}
