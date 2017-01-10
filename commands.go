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
		setupEgressCommands(),
		setupIngressCommands(),
	}

	return app
}
