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
		},
		setupEgressCommands(),
		setupIngressCommands(),
	}

	return app
}
