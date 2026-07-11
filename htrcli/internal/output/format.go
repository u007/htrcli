package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
)

// JSONOutput controls whether to output raw JSON.
var JSONOutput bool

// PrintJSON prints data as formatted JSON.
func PrintJSON(data any) {
	jsonBytes, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error marshaling JSON: %v\n", err)
		os.Exit(1)
	}
	fmt.Println(string(jsonBytes))
}

// PrintJSONRaw prints raw JSON bytes.
func PrintJSONRaw(data []byte) {
	var pretty bytes.Buffer
	if err := json.Indent(&pretty, data, "", "  "); err != nil {
		fmt.Println(string(data))
		return
	}
	fmt.Println(pretty.String())
}

// PrintOrJSON outputs as JSON if --json flag is set, otherwise calls the formatter.
func PrintOrJSON(jsonData any, humanFormatter func()) {
	if JSONOutput {
		PrintJSON(jsonData)
	} else {
		humanFormatter()
	}
}

// PrintResult prints a command result with timing.
func PrintResult(action string, result interface{ GetDuration() int }, extra ...string) {
	if JSONOutput {
		PrintJSON(result)
		return
	}
	msg := fmt.Sprintf("%s (%dms)", action, result.GetDuration())
	if len(extra) > 0 {
		msg = extra[0]
	}
	fmt.Println(msg)
}
