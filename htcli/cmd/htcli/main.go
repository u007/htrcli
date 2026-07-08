package main

import (
	"os"
	"strings"

	"github.com/u007/htcli/internal/commands"
	"github.com/u007/htcli/internal/host"
)

// hostManifestName is the native-messaging manifest filename. Firefox passes
// the full path to this manifest as the first argument when spawning the host.
const hostManifestName = "com.howtorecorder.host.json"

// firefoxExtensionID is the add-on ID Firefox passes as the second argument.
const firefoxExtensionID = "htrcontrol@mercstudio.com"

// isNativeHostLaunch reports whether the browser spawned us as a native
// messaging host. Detection differs per browser:
//   - Chrome passes the extension origin "chrome-extension://<id>/" as argv[1].
//   - Firefox passes the full path to the app manifest as argv[1] and the
//     add-on ID as argv[2].
//
// We only inspect the native-messaging positions, not every argument, so a
// normal CLI invocation like `htcli install --extension-id htrncontrol@...`
// does not get misclassified as a browser launch.
//
// Misdetecting this matters: if we fall through to the cobra CLI, its help/error
// text is written to stdout, which the browser reads as a (corrupt) native
// message length prefix — producing a multi-hundred-MB "message" that exceeds
// the 1 MB native-messaging limit and aborts the connection.
func isNativeHostLaunch() bool {
	if len(os.Args) > 1 {
		arg := os.Args[1]
		if strings.HasPrefix(arg, "chrome-extension://") || strings.HasSuffix(arg, hostManifestName) {
			return true
		}
	}
	if len(os.Args) > 2 && os.Args[2] == firefoxExtensionID {
		return true
	}
	return false
}

func main() {
	if isNativeHostLaunch() {
		if err := host.RunRelay(); err != nil {
			os.Exit(1)
		}
		return
	}
	commands.Execute()
}
