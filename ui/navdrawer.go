package ui

import (
	"github.com/hajimehoshi/ebiten/v2"
)

// NavDrawer slide out modal menu
type NavDrawer struct {
	DrawerBase
}

// NewNavDrawer creates the NavDrawer object; it starts life off screen to the left
func NewNavDrawer() *NavDrawer {
	// according to https://material.io/components/navigation-drawer#specs, always 256 wide
	n := &NavDrawer{DrawerBase: DrawerBase{width: 256, height: 0, x: -256, y: 48}}
	n.widgets = []Widgety{
		// widget x, y will be set by LayoutWidgets()
		NewNavItem(n, "newDeal", "star", "New deal", ebiten.KeyN),
		NewNavItem(n, "restartDeal", "restore", "Restart deal", ebiten.KeyR),
		NewNavItem(n, "findGame", "search", "Find game...", ebiten.KeyF),
		NewNavItem(n, "bookmark", "bookmark_add", "Bookmark", ebiten.KeyS),
		NewNavItem(n, "gotoBookmark", "bookmark", "Goto bookmark", ebiten.KeyL),
		NewNavItem(n, "wikipedia", "info", "Wikipedia...", ebiten.KeyF1),
		NewNavItem(n, "statistics", "list", "Statistics", ebiten.KeyF2),
		NewNavItem(n, "settings", "settings", "Settings...", ebiten.KeyF3),
	}
	// don't know how to ask a browser window to close
	// if runtime.GOARCH != "wasm" {
	// 	n.widgets = append(n.widgets, NewNavItem(n, "close", "Save and exit", ebiten.KeyX))
	// }
	n.LayoutWidgets()
	return n
}

// ToggleNavDrawer animates the drawer on/off screen to the left
func (u *UI) ToggleNavDrawer() {

	con := u.VisibleDrawer()
	if con == u.navDrawer {
		con.Hide()
		return
	}
	if con == nil {
		u.navDrawer.Show()
		return
	}
	con.Hide()
	u.navDrawer.Show()
}
