package main

import (
	"errors"
	"flag"
	"io"
	"os"

	"github.com/rz1998/invest-md-uju/rpc/mdCurrent/internal/logic/snappy"
)

var (
	decode = flag.Bool("d", false, "decode")
	encode = flag.Bool("e", false, "encode")
)

func run() error {
	flag.Parse()
	if *decode == *encode {
		return errors.New("exactly one of -d or -e must be given")
	}

	in, err := io.ReadAll(os.Stdin)
	if err != nil {
		return err
	}

	out := []byte(nil)
	if *decode {
		out, err = snappy.Decode(nil, in)
		if err != nil {
			return err
		}
	} else {
		out = snappy.Encode(nil, in)
	}
	_, err = os.Stdout.Write(out)
	return err
}

func main() {
	if err := run(); err != nil {
		os.Stderr.WriteString(err.Error() + "\n")
		os.Exit(1)
	}
}
