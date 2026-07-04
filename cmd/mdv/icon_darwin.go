package main

/*
#cgo CFLAGS: -x objective-c
#cgo LDFLAGS: -framework Cocoa -framework UniformTypeIdentifiers
#import <Cocoa/Cocoa.h>
static void mdvSetDockIcon(const void *data, int len) {
	NSImage *src = [[NSImage alloc] initWithData:[NSData dataWithBytes:data length:len]];
	// qlmanage renders icon.svg onto opaque white, so clip to the tile's
	// rounded rect (48,48 928x928 r=220 in the SVG's 1024 grid) to get
	// transparent corners in the dock.
	NSImage *img = [NSImage imageWithSize:src.size flipped:NO drawingHandler:^BOOL(NSRect rect) {
		CGFloat s = rect.size.width / 1024.0;
		[[NSBezierPath bezierPathWithRoundedRect:NSMakeRect(48*s, 48*s, 928*s, 928*s)
		                                 xRadius:220*s yRadius:220*s] addClip];
		[src drawInRect:rect];
		return YES;
	}];
	NSApp.applicationIconImage = img;
}
*/
import "C"

import (
	_ "embed"
	"unsafe"
)

//go:embed icon.png
var iconPNG []byte

// setDockIcon replaces the generic "exec" dock icon. Must run after
// webview.New has initialized NSApplication.
func setDockIcon() {
	C.mdvSetDockIcon(unsafe.Pointer(&iconPNG[0]), C.int(len(iconPNG)))
}
