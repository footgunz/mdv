package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers
#import <Cocoa/Cocoa.h>
static void mdvSetDockIcon(const void *data, int len) {
	NSApp.applicationIconImage = [[NSImage alloc] initWithData:[NSData dataWithBytes:data length:len]];
}
*/
import "C"

import (
	_ "embed"
	"unsafe"
)

//go:embed icon.png
var iconPNG []byte

// setDockIcon replaces the generic "exec" dock icon when mdv runs as a bare
// binary. Must run after webview.New has initialized NSApplication. Inside
// mdv.app the bundle's own .icns wins and this is a harmless no-op.
func setDockIcon() {
	C.mdvSetDockIcon(unsafe.Pointer(&iconPNG[0]), C.int(len(iconPNG)))
}
