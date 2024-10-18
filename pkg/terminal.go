package pkg

import (
    "fmt"
    "os"
    "os/exec"
    "path/filepath"
    "runtime"
    "slices"
    "strings"

    d "github.com/Karmenzind/kd/pkg/decorate"
    "github.com/google/shlex"
    "go.uber.org/zap"
    "golang.org/x/term"
)

var EXECUTABLE_BASENAME string
var EXECUTABLE_PATH string
var term_height int
var term_width int

func fetchExecutableInfo() error {
    kdpath, err := os.Executable()
    if err == nil {
        EXECUTABLE_PATH = kdpath
        EXECUTABLE_BASENAME = filepath.Base(kdpath)

    }
    return err

}

func GetExecutablePath() (string, error) {
    var err error
    if EXECUTABLE_PATH == "" {
        err = fetchExecutableInfo()
    }
    return EXECUTABLE_PATH, err
}

func GetExecutableBasename() (string, error) {
    var err error
    if EXECUTABLE_BASENAME == "" {
        err = fetchExecutableInfo()
    }
    return EXECUTABLE_BASENAME, err
}

// pager or print
// config > $PAGER > less -f
// TODO 增加检测pager可用
func OutputResult(out string, paging bool, pagerCmd string, doClear bool) error {
    var err error
    var logger = zap.S()
    if paging {
        if pagerCmd == "" {
            // XXX (k): <2023-12-31> expandenv?
            if sysPager := os.Getenv("PAGER"); sysPager != "" {
                logger.Debugf("Using system pager %s", sysPager)
                pagerCmd = sysPager
            } else if runtime.GOOS != "windows" && CommandExists("less") {
                pagerCmd = "less -RF"
                logger.Debugf("Using default pager %s", pagerCmd)
            }
        } else {
            logger.Debugf("Using assigned pager `%s`", pagerCmd)
        }

        if pagerCmd != "" {
            var pager *exec.Cmd
            var program string
            if strings.Contains(pagerCmd, " ") {
                args, serr := shlex.Split(pagerCmd)
                program = args[0]
                if serr != nil {
                    return err
                }
                pager = exec.Command(args[0], args[1:]...)
            } else {
                program = pagerCmd
                pager = exec.Command(pagerCmd)
            }
            if CommandExists(program) {
                // pager.Stdin = strings.NewReader(out)
                pager.Stdout = os.Stdout
                pager.Stderr = os.Stderr
                err = Output2PagerVer2(pager, out)
                // err = pager.Run()
                return err
            }
            d.EchoWarn(fmt.Sprintf("pager command `%s` not found", program))
        }
    }
    if doClear {
        _, h, err := GetTermSize()
        if err == nil && strings.Count(out, "\n") < h {
            ClearScreen()
        }
    }
    fmt.Println(out)
    return nil
}

func Output2PagerVer1(pager *exec.Cmd, output string) (err error) {
    pager.Stdin = strings.NewReader(output)
    err = pager.Run()

    return err
}

func Output2PagerVer2(pager *exec.Cmd, output string) (err error) {
    pipe, err := pager.StdinPipe()
    if err != nil {
        return
    }

    if err = pager.Start(); err != nil {
        return err
    }

    defer func() {
        pipe.Close()
        pager.Wait()
    }()
    fmt.Fprintln(pipe, output)
    return err
}

func CommandExists(cmd string) bool {
    p, err := exec.LookPath(cmd)
    zap.S().Debugf("Got path of %s: %v", cmd, p)
    return err == nil
}

// ask yes or no
func AskYN(prompt string) bool {
    var input string
    for {
        fmt.Print(d.Blue(":: "), prompt, " [Y/n] ")
        fmt.Scanln(&input)
        switch input {
        case "", "Y", "y":
            return true
        case "n":
            return false
        default:
            fmt.Println("Please input Y or n.")
            continue
        }
    }
}

func ClearScreen() {
    var c *exec.Cmd
    switch runtime.GOOS {
    case "linux", "darwin":
        c = exec.Command("clear")
    case "windows":
        c = exec.Command("cls")
    }
    c.Stdout = os.Stdout
    c.Run()
    zap.S().Debugf("Cleared screen.")
}

func GetTermSize() (int, int, error) {
    if term_height > 0 && term_width > 0 {
        return term_width, term_height, nil
    }
    w, h, err := term.GetSize(0)
    if err != nil {
        return 0, 0, err
    }
    term_height = h
    term_width = w
    return w, h, nil
}

func HasAnyFlag(flags ...string) bool {
    for idx := range flags {
        if slices.Index(os.Args, "--"+flags[idx]) > 0 {
            return true
        }
    }
    return false
}
