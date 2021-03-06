package common

import (
	"fmt"
	"os"
	"strconv"

	"github.com/elastos/Elastos.ELA/common/config"

	"github.com/urfave/cli"
)

func LocalServer() string {
	return "http://localhost" + ":" + strconv.Itoa(config.Parameters.HttpJsonPort)
}

func PrintError(c *cli.Context, err error, cmd string) {
	fmt.Println("Incorrect Usage:", err)
	fmt.Println("")
	cli.ShowCommandHelp(c, cmd)
}

func FileExisted(filename string) bool {
	_, err := os.Stat(filename)
	return err == nil || os.IsExist(err)
}
