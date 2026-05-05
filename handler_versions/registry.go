// Package versions provides version-specific network packet handling.
//
// This package imports all supported version implementations, triggering their
// init() functions to register with the common.Factory. Import this package
// to enable version handler lookup via common.GetVersionHandler().
//
// Example usage:
//
//	import (
//	    "github.com/reallyoldfogie/mc-agent/handler_versions"
//	    "github.com/reallyoldfogie/mc-agent/handler_versions/common"
//	)
//
//	func main() {
//	    _ = versions.Supported // Ensure import
//	    handler, err := common.GetVersionHandler("1.21.5")
//	    if err != nil {
//	        log.Fatal(err)
//	    }
//	    // Use handler...
//	}
package versions
