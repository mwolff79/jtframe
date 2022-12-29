/*  This file is part of JT_FRAME.
    JTFRAME program is free software: you can redistribute it and/or modify
    it under the terms of the GNU General Public License as published by
    the Free Software Foundation, either version 3 of the License, or
    (at your option) any later version.

    JTFRAME program is distributed in the hope that it will be useful,
    but WITHOUT ANY WARRANTY; without even the implied warranty of
    MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
    GNU General Public License for more details.

    You should have received a copy of the GNU General Public License
    along with JTFRAME.  If not, see <http://www.gnu.org/licenses/>.

    Author: Jose Tejada Gomez. Twitter: @topapate
    Date: 28-8-20122 */

package cmd

import (
	"os"
	"github.com/jotego/jtframe/mra"

	"github.com/spf13/cobra"
)

var mra_args mra.Args

// mraCmd represents the mra command
var mraCmd = &cobra.Command{
	Use:   "mra <core-name>",
	Short: "Parses the core's TOML file to generate MRA files",
	Long: `Parses the core's mame2mra.toml file to generate MRA files.

TOML elements (see full reference in mame2mra.go)

[ROM]
regions = [
	{ name=maincpu, machine=optional, start=0, width=16, len=0x10000, reverse=true },
	{ name==soundcpu, sequence=[2,1,0,0] } # inverts the order and repeats the first ROM
	{ name=plds, skip=true },
]
`,
	Run: func(cmd *cobra.Command, args []string) {
		mra_args.Def_cfg.Core = args[0]
		// mra_args.Toml_path = args[0] + ".toml"

		mra.Run(mra_args)
	},
	Args: cobra.ExactArgs(1),
}

func init() {
	rootCmd.AddCommand(mraCmd)
	flag := mraCmd.Flags()

	mra_args.Def_cfg.Target = "mist"
	flag.StringVar(&mra_args.Def_cfg.Commit, "commit", "", "result of running 'git rev-parse --short HEAD'")
	flag.StringVar(&mra_args.Xml_path, "xml", os.Getenv("JTROOT")+"/rom/mame.xml", "Path to MAME XML file")
	flag.StringVar(&mra_args.Year, "year", "", "Year string for MRA file comment")
	flag.BoolVarP(&mra_args.Verbose, "verbose", "v", false, "verbose")
	flag.BoolVarP(&mra_args.SkipMRA, "skipMRA", "s", false, "Do not generate MRA files")
	flag.BoolVarP(&mra_args.Show_platform, "show_platform", "p", false, "Show platform name and quit")
	flag.BoolVarP(&mra_args.JTbin, "git", "g", false, "Save files to JTBIN")
	flag.StringVar(&mra_args.Buttons, "buttons", "", "Buttons used by the game -upto six-")
	flag.StringVar(&mra_args.Author, "author", "jotego", "Core author")
	flag.StringVar(&mra_args.URL, "url", "https://patreon.com/jotego", "Author's URL")
}
