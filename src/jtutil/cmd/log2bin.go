/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>

*/
package cmd

import (
	"bufio"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"github.com/spf13/cobra"
)

var flags struct{
	a0, size int
	verbose bool
	fout string
}

// log2binCmd represents the log2bin command
var log2binCmd = &cobra.Command{
	Use:   "log2bin",
	Short: "Converts debug.log, made of address=data line to a binary file",
	Long: `Generate the input file with the wpset command of the MAME debugger.
Use -a0 and -s to specify the beginning of the region and its size
`,
	Run: log2bin,
}

func init() {
	rootCmd.AddCommand(log2binCmd)

	// Here you will define your flags and configuration settings.

	// Cobra supports Persistent Flags which will work for this command
	// and all subcommands, e.g.:
	// log2binCmd.PersistentFlags().String("foo", "", "A help for foo")

	// Cobra supports local flags which will only run when this command
	// is called directly, e.g.:
	// log2binCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
	flg  := log2binCmd.Flags()
	flg.IntVarP(&flags.a0,"start","a", 0, "address start of region")
	flg.IntVarP(&flags.size,"size","s", 0x400, "region length")
	flg.BoolVarP(&flags.verbose,"verbose","v", false, "verbose")
	flg.StringVarP(&flags.fout,"output","o", "debug.bin", "name of output file")

}

func log2bin(cmd *cobra.Command, args []string) {
	fname := "debug.log"
	if len(args) == 1 {
		fname = args[0]
	}
	f, e := os.Open(fname)
	if e != nil {
		log.Fatal(e)
	}
	defer f.Close()
	if flags.verbose {
		fmt.Printf("Parsing %s for writes to %X (%X long)\n",
			fname, flags.a0, flags.size )
	}
	// Prepare the buffer
	mem := make([]byte, flags.size)

	scn := bufio.NewScanner(f)
	lcnt, lparsed := 0, 0
	for lcnt = 0; scn.Scan(); lcnt++ {
		tokens := strings.Split(scn.Text(), "=")
		if len(tokens) != 2 {
			continue
		}
		a, e := strconv.ParseInt(tokens[0], 16, 32)
		if e != nil {
			continue
		}
		a -= int64(flags.a0)
		if a > int64(flags.size) || a < 0 {
			continue
		}
		d, e := strconv.ParseInt(tokens[1], 16, 16)
		if e != nil {
			continue
		}
		mem[a] = byte(d)
		lparsed++
	}
	// Dump the buffer
	e = os.WriteFile(flags.fout, mem, 0666)
	if e != nil {
		log.Fatal(e)
	}
	fmt.Printf("Parsed %d lines of %d\n", lparsed, lcnt)
}