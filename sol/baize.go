package sol

import (
	"fmt"
	"hash/crc32"
	"image"
	"log"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/inpututil"
	"oddstream.games/gosol/input"
	"oddstream.games/gosol/sound"
	"oddstream.games/gosol/ui"
	"oddstream.games/gosol/util"
)

const (
	BAIZEMAGIC uint32 = 0xfeedface
)

const (
	dirtyWindowSize = 1 << iota
	dirtyPilePositions
	dirtyCardSizes
	dirtyCardImages
	dirtyPileBackgrounds
	dirtyCardPositions
)

// Baize object describes the baize
type Baize struct {
	magic            uint32
	script           Scripter
	piles            []*Pile
	tail             []*Card // array of cards currently being dragged
	bookmark         int     // index into undo stack
	recycles         int     // number of available stock recycles
	undoStack        []*SavableBaize
	dirtyFlags       uint32 // what needs doing when we Update
	moves            int    // number of possible (not useless) moves
	fmoves           int    // number of possible moves to a Foundation (for enabling Collect button)
	stroke           *input.Stroke
	dragStart        image.Point
	dragOffset       image.Point
	showMovableCards bool // show movable cards until the next move (default: false)
	WindowWidth      int  // the most recent window width given to Layout
	WindowHeight     int  // the most recent window height given to Layout
}

//--+----1----+----2----+----3----+----4----+----5----+----6----+----7----+----8

// NewBaize is the factory func for the single Baize object
func NewBaize() *Baize {
	// let WindowWidth,WindowHeight be zero, so that the first Layout will trigger card scaling and pile placement
	return &Baize{magic: BAIZEMAGIC, dragOffset: image.Point{0, 0}, dirtyFlags: 0xFFFF}
}

func (b *Baize) flagSet(flag uint32) bool {
	return b.dirtyFlags&flag == flag
}

func (b *Baize) setFlag(flag uint32) {
	b.dirtyFlags |= flag
}

func (b *Baize) clearFlag(flag uint32) {
	b.dirtyFlags &= ^flag
}

func (b *Baize) Valid() bool {
	return b != nil && b.magic == BAIZEMAGIC
}

func (b *Baize) CRC() uint32 {
	/*
		var crc uint = 0xFFFFFFFF
		var mask uint
		for _, p := range b.piles {
			crc = crc ^ uint(p.Len())
			for j := 7; j >= 0; j-- {
				mask = -(crc & 1)
				crc = (crc >> 1) ^ (0xEDB88320 & mask)
			}
		}
		return ^crc // bitwise NOT
	*/
	var lens []byte
	for _, p := range b.piles {
		lens = append(lens, byte(p.Len()))
	}
	return crc32.ChecksumIEEE(lens)
}

func (b *Baize) AddPile(pile *Pile) {
	b.piles = append(b.piles, pile)
}

func (b *Baize) Refan() {
	b.setFlag(dirtyCardPositions)
}

func (b *Baize) LongVariantName() string {
	return ThePreferences.Variant
}

// NewGame restarts current variant (ie no pile building) with a new seed
func (b *Baize) NewDeal() {

	b.StopSpinning()

	// a virgin game has one state on the undo stack
	if len(b.undoStack) > 1 && !b.Complete() {
		TheStatistics.RecordLostGame(b.LongVariantName())
	}

	b.Reset()
	for _, p := range b.piles {
		p.Reset()
	}

	stockPile := b.script.Stock()
	FillFromLibrary(stockPile)
	stockPile.Shuffle()

	b.showMovableCards = false
	b.script.StartGame()
	b.UndoPush()
	b.FindDestinations()
	b.UpdateToolbar()
	b.UpdateStatusbar()

	sound.Play("Fan")

	b.setFlag(dirtyCardPositions)
}

func (b *Baize) ShowVariantGroupPicker() {
	TheUI.ShowVariantGroupPicker(VariantGroupNames())
}

func (b *Baize) ShowVariantPicker(group string) {
	TheUI.ShowVariantPicker(VariantNames(group))
}

func (b *Baize) MirrorSlots() {
	/*
		0 1 2 3 4 5
		5 4 3 2 1 0

		0 1 2 3 4
		4 3 2 1 0
	*/
	var minX int = 32767
	var maxX int = 0
	for _, p := range b.piles {
		if p.Slot().X < 0 {
			continue // ignore hidden pile
		}
		if p.Slot().X < minX {
			minX = p.Slot().X
		}
		if p.Slot().X > maxX {
			maxX = p.Slot().X
		}
	}
	for _, p := range b.piles {
		slot := p.Slot()
		if slot.X < 0 {
			continue // ignore hidden pile
		}
		p.SetSlot(image.Point{X: maxX - slot.X + minX, Y: slot.Y})
		switch p.FanType() {
		case FAN_RIGHT:
			p.SetFanType(FAN_LEFT)
		case FAN_LEFT:
			p.SetFanType(FAN_RIGHT)
		case FAN_RIGHT3:
			p.SetFanType(FAN_LEFT3)
		case FAN_LEFT3:
			p.SetFanType(FAN_RIGHT3)
		}
	}
}

func (b *Baize) Reset() {
	b.tail = nil
	b.undoStack = nil
	b.bookmark = 0
	MarkAllCardsImmovable()
}

// StartFreshGame resets Baize and starts a new game with a new seed
func (b *Baize) StartFreshGame() {

	b.Reset()
	b.piles = nil

	var ok bool
	if b.script, ok = Variants[ThePreferences.Variant]; !ok {
		log.Println("no interface for variant", ThePreferences.Variant)
		ThePreferences.Variant = "Klondike"
		ThePreferences.Save()
		if b.script, ok = Variants[ThePreferences.Variant]; !ok {
			log.Panic("no interface for Klondike")
		}
		NoGameLoad = true
	}
	b.script.BuildPiles()
	if ThePreferences.MirrorBaize {
		b.MirrorSlots()
	}
	// b.FindBuddyPiles()

	TheUI.SetTitle(b.LongVariantName())

	sound.Play("Fan")

	b.dirtyFlags = 0xFFFF

	b.showMovableCards = false
	b.script.StartGame()
	b.UndoPush()
	b.FindDestinations()
	b.UpdateToolbar()
	b.UpdateStatusbar()
}

func (b *Baize) ChangeVariant(newVariant string) {
	// a virgin game has one state on the undo stack
	if len(b.undoStack) > 1 && !b.Complete() {
		TheStatistics.RecordLostGame(b.LongVariantName())
	}
	ThePreferences.Variant = newVariant
	b.StartFreshGame()
}

func (b *Baize) SetUndoStack(undoStack []*SavableBaize) {
	b.undoStack = undoStack
	sav := b.UndoPeek()
	b.UpdateFromSavable(sav)
	b.FindDestinations()
	if b.Complete() {
		TheUI.Toast("Complete", "Complete")
		TheUI.ShowFAB("star", ebiten.KeyN)
		b.StartSpinning()
	} else if b.Conformant() {
		TheUI.ShowFAB("done_all", ebiten.KeyC)
	} else if b.moves == 0 {
		TheUI.Toast("", "No movable cards")
		TheUI.ShowFAB("star", ebiten.KeyN)
	} else {
		TheUI.HideFAB()
	}
	b.UpdateToolbar()
	b.UpdateStatusbar()
}

// findPileAt finds the Pile under the mouse position
func (b *Baize) FindPileAt(pt image.Point) *Pile {
	for _, p := range b.piles {
		if pt.In(p.ScreenRect()) {
			return p
		}
	}
	return nil
}

// FindLowestCardAt finds the bottom-most Card under the mouse position
func (b *Baize) FindLowestCardAt(pt image.Point) *Card {
	for _, p := range b.piles {
		for i := p.Len() - 1; i >= 0; i-- {
			c := p.Get(i)
			if pt.In(c.ScreenRect()) {
				return c
			}
		}
	}
	return nil
}

// FindHighestCardAt finds the top-most Card under the mouse position
func (b *Baize) FindHighestCardAt(pt image.Point) *Card {
	for _, p := range b.piles {
		for _, c := range p.cards {
			if pt.In(c.ScreenRect()) {
				return c
			}
		}
	}
	return nil
}

func (b *Baize) LargestIntersection(c *Card) *Pile {
	var largestArea int = 0
	var pile *Pile = nil
	cardRect := c.BaizeRect()
	for _, p := range b.piles {
		if p == c.Owner() {
			continue
		}
		pileRect := p.FannedBaizeRect()
		intersectRect := pileRect.Intersect(cardRect)
		area := intersectRect.Dx() * intersectRect.Dy()
		if area > largestArea {
			largestArea = area
			pile = p
		}
	}
	return pile
}

// StartDrag return true if the Baize can be dragged
func (b *Baize) StartDrag() bool {
	b.dragStart = b.dragOffset
	return true
}

// DragBy move ('scroll') the Baize by dragging it
// dx, dy is the difference between where the drag started and where the cursor is now
func (b *Baize) DragBy(dx, dy int) {
	b.dragOffset.X = b.dragStart.X + dx
	if b.dragOffset.X > 0 {
		b.dragOffset.X = 0 // DragOffsetX should only ever be 0 or -ve
	}
	b.dragOffset.Y = b.dragStart.Y + dy
	if b.dragOffset.Y > 0 {
		b.dragOffset.Y = 0 // DragOffsetY should only ever be 0 or -ve
	}
}

// StopDrag stop dragging the Baize
func (b *Baize) StopDrag() {
	b.setFlag(dirtyCardPositions)
}

// StartSpinning tells all the cards to start spinning
func (b *Baize) StartSpinning() {
	for _, p := range b.piles {
		// use a method expression, which yields a function value with a regular first parameter taking the place of the receiver
		p.ApplyToCards((*Card).StartSpinning)
	}
}

// StopSpinning tells all the cards to stop spinning and return to their upright position
// debug only
func (b *Baize) StopSpinning() {
	for _, p := range b.piles {
		// use a method expression, which yields a function value with a regular first parameter taking the place of the receiver
		p.ApplyToCards((*Card).StopSpinning)
	}
	b.setFlag(dirtyCardPositions)
}

func (b *Baize) MakeTail(c *Card) bool {
	b.tail = c.Owner().MakeTail(c)
	return len(b.tail) > 0
}

func (b *Baize) AfterUserMove() {
	b.showMovableCards = false
	b.script.AfterMove()
	b.UndoPush()
	b.FindDestinations()
	b.UpdateToolbar()
	b.UpdateStatusbar()

	if b.Complete() {
		TheUI.ShowFAB("star", ebiten.KeyN)
		b.StartSpinning()
		TheStatistics.RecordWonGame(b.LongVariantName(), len(b.undoStack)-1)
	} else if b.Conformant() {
		TheUI.ShowFAB("done_all", ebiten.KeyC)
	} else if b.moves == 0 {
		TheUI.ToastError("No movable cards")
		TheUI.ShowFAB("star", ebiten.KeyN)
	} else {
		TheUI.HideFAB()
	}
}

/*
InputStart finds out what object the user input is starting on
(UI Container > Card > Pile > Baize, in that order)
then tells that object.

If the Input starts on a Card, then a tail of cards is formed.
*/
func (b *Baize) InputStart(v input.StrokeEvent) {
	b.stroke = v.Stroke

	if con := TheUI.FindContainerAt(v.X, v.Y); con != nil {
		if con.StartDrag(b.stroke) {
			b.stroke.SetDraggedObject(con)
		} else {
			b.stroke.Cancel()
		}
	} else {
		pt := image.Pt(v.X, v.Y)
		if c := b.FindLowestCardAt(pt); c != nil {
			// currently don't allow transitioning cards to be dragged
			// see Card.StartDrag()
			if c.Transitioning() {
				// TheUI.ToastError("Cannot drag a moving card")
				sound.Play("Error")
			} else {
				b.StartTailDrag(c)
				b.stroke.SetDraggedObject(c)
			}
		} else {
			if p := b.FindPileAt(pt); p != nil {
				b.stroke.SetDraggedObject(p)
			} else {
				if b.StartDrag() {
					b.stroke.SetDraggedObject(b)
				} else {
					v.Stroke.Cancel()
				}
			}
		}
	}
}

func (b *Baize) InputMove(v input.StrokeEvent) {
	if v.Stroke.DraggedObject() == nil {
		return
		// log.Panic("*** move stroke with nil dragged object ***")
	}
	// for _, p := range b.piles {
	// 	p.target = false
	// }
	switch v.Stroke.DraggedObject().(type) {
	case ui.Containery:
		con := v.Stroke.DraggedObject().(ui.Containery)
		con.DragBy(v.Stroke.PositionDiff())
	case *Card:
		b.DragTailBy(v.Stroke.PositionDiff())
		// if c, ok := v.Stroke.DraggedObject().(*Card); ok {
		// 	if p := b.LargestIntersection(c); p != nil {
		// 		p.target = true
		// 	}
		// }
	case *Pile:
		// do nothing
	case *Baize:
		b.DragBy(v.Stroke.PositionDiff())
	default:
		log.Panic("*** unknown move dragging object ***")
	}
}

func (b *Baize) InputStop(v input.StrokeEvent) {
	if v.Stroke.DraggedObject() == nil {
		return
		// log.Panic("*** stop stroke with nil dragged object ***")
	}
	// for _, p := range b.piles {
	// 	p.SetTarget(false)
	// }
	switch v.Stroke.DraggedObject().(type) {
	case ui.Containery:
		con := v.Stroke.DraggedObject().(ui.Containery)
		con.StopDrag()
	case *Card:
		c := v.Stroke.DraggedObject().(*Card)
		if c.WasDragged() {
			src := c.Owner()
			// tap handled elsewhere
			// tap is time-limited
			if dst := b.LargestIntersection(c); dst == nil {
				// println("no intersection for", c.String())
				b.CancelTailDrag()
			} else {
				var ok bool
				var err error
				// generically speaking, can this tail be moved?
				if ok, err = src.CanMoveTail(b.tail); !ok {
					TheUI.ToastError(err.Error())
					b.CancelTailDrag()
				} else {
					if ok, err = dst.vtable.CanAcceptTail(b.tail); !ok {
						TheUI.ToastError(err.Error())
						b.CancelTailDrag()
					} else {
						// it's ok to move this tail
						if src == dst {
							b.CancelTailDrag()
						} else if ok, err = b.script.TailMoveError(b.tail); !ok {
							TheUI.ToastError(err.Error())
							b.CancelTailDrag()
						} else {
							crc := b.CRC()
							if len(b.tail) == 1 {
								MoveCard(src, dst)
							} else {
								MoveTail(c, dst)
							}
							if crc != b.CRC() {
								b.AfterUserMove()
							}
							b.StopTailDrag()
						}
					}
				}
			}
		}
	case *Pile:
		// do nothing
	case *Baize:
		// println("stop dragging baize")
		b.StopDrag()
	default:
		log.Panic("*** stop dragging unknown object ***")
	}
}

func (b *Baize) InputCancel(v input.StrokeEvent) {
	if v.Stroke.DraggedObject() == nil {
		return
		// log.Panic("*** cancel stroke with nil dragged object ***")
	}
	switch v.Stroke.DraggedObject().(type) { // type switch
	case ui.Containery:
		con := v.Stroke.DraggedObject().(ui.Containery)
		con.StopDrag()
	case *Card:
		b.CancelTailDrag()
	case *Pile:
		// p := v.Stroke.DraggedObject().(*Pile)
		// println("stop dragging pile", p.Class)
		// do nothing
	case *Baize:
		// println("stop dragging baize")
		b.StopDrag()
	default:
		log.Panic("*** cancel dragging unknown object ***")
	}
}

func (b *Baize) InputTap(v input.StrokeEvent) {
	// println("Baize.NotifyCallback() tap", v.X, v.Y)
	switch obj := v.Stroke.DraggedObject().(type) {
	case *Card:
		// offer TailTapped to the script first
		// to implement things like Stock.TailTapped
		// if the script doesn't want to do anything, it can call pile.vtable.TailTapped
		// which will either ignore it (eg Foundation, Discard)
		// or use Pile.DefaultTailTapped
		crc := b.CRC()
		b.script.TailTapped(b.tail)
		if crc != b.CRC() {
			sound.Play("Slide")
			b.AfterUserMove()
		}
		b.StopTailDrag()
	case *Pile:
		crc := b.CRC()
		b.script.PileTapped(obj)
		if crc != b.CRC() {
			sound.Play("Shove")
			b.AfterUserMove()
		}
	case *Baize:
		pt := image.Pt(v.X, v.Y)
		// a tap outside any open ui drawer (ie on the baize) closes the drawer
		if con := TheUI.VisibleDrawer(); con != nil && !pt.In(image.Rect(con.Rect())) {
			con.Hide()
		}
	}
}

// NotifyCallback is called by the Subject (Input/Stroke) when something interesting happens
func (b *Baize) NotifyCallback(v input.StrokeEvent) {
	switch v.Event {
	case input.Start:
		b.InputStart(v)
	case input.Move:
		b.InputMove(v)
	case input.Stop:
		b.InputStop(v)
	case input.Cancel:
		b.InputCancel(v)
	case input.Tap:
		b.InputTap(v)
	default:
		log.Panic("*** unknown stroke event ***", v.Event)
	}
}

// ApplyToTail applies a method func to this card and all the others after it in the tail
func (b *Baize) ApplyToTail(fn func(*Card)) {
	// https://golang.org/ref/spec#Method_expressions
	// (*Card).CancelDrag yields a function with the signature func(*Card)
	// fn passed as a method expression so add the receiver explicitly
	for _, c := range b.tail {
		fn(c)
	}
}

// DragTailBy repositions all the cards in the tail: dx, dy is the position difference from the start of the drag
func (b *Baize) DragTailBy(dx, dy int) {
	// println("Baize.DragTailBy(", dx, dy, ")")
	for _, c := range b.tail {
		c.DragBy(dx, dy)
	}
}

func (b *Baize) StartTailDrag(c *Card) {
	if b.MakeTail(c) {
		b.ApplyToTail((*Card).StartDrag)
		ebiten.SetCursorMode(ebiten.CursorModeHidden)
	} else {
		println("failed to make a tail")
	}
}

func (b *Baize) StopTailDrag() {
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
	b.ApplyToTail((*Card).StopDrag)
	b.tail = nil
}

func (b *Baize) CancelTailDrag() {
	ebiten.SetCursorMode(ebiten.CursorModeVisible)
	b.ApplyToTail((*Card).CancelDrag)
	b.tail = nil
}

// collectFromPile is a helper function for Collect2()
func (b *Baize) collectFromPile(pile *Pile) int {
	if pile == nil {
		return 0
	}
	var cardsMoved int = 0
	for _, fp := range b.script.Foundations() {
		for {
			var card *Card = pile.Peek()
			if card == nil {
				return cardsMoved
			}
			ok, _ := fp.vtable.CanAcceptTail([]*Card{card})
			if !ok {
				break // done with this foundation, try another
			}
			MoveCard(pile, fp)
			b.AfterUserMove() // does an undoPush()
			cardsMoved += 1
		}
	}
	return cardsMoved
}

// Collect2 should be exactly the same as the user tapping repeatedly on the
// waste, cell, reserve and tableau piles
// nb there is no collecting to discard piles, they are optional and presence of
// cards in them does not signify a complete game
func (b *Baize) Collect2() {
	for {
		var totalCardsMoved int = b.collectFromPile(b.script.Waste())
		for _, pile := range b.script.Cells() {
			totalCardsMoved += b.collectFromPile(pile)
		}
		for _, pile := range b.script.Reserves() {
			totalCardsMoved += b.collectFromPile(pile)
		}
		for _, pile := range b.script.Tableaux() {
			totalCardsMoved += b.collectFromPile(pile)
		}
		if totalCardsMoved == 0 {
			break
		}
	}
}

func (b *Baize) MaxSlotX() int {
	var maxX int
	for _, p := range b.piles {
		if p.Slot().X > maxX {
			maxX = p.Slot().X
		}
	}
	return maxX
}

// ScaleCards calculates new width/height of cards and margins
// returns true if changes were made
func (b *Baize) ScaleCards() bool {

	// const (
	// 	DefaultRatio = 1.444
	// 	BridgeRatio  = 1.561
	// 	PokerRatio   = 1.39
	// 	OpsoleRatio  = 1.5556 // 3.5/2.25
	// )

	var OldWidth = CardWidth
	var OldHeight = CardHeight

	var maxX int = b.MaxSlotX()

	// "add" two extra piles and a LeftMargin to make a half-card-width border

	/*
		71 x 96 = 1:1.352 (Microsoft retro)
		140 x 190 = 1:1.357 (kenney, large)
		64 x 89 = 1:1.390 (official poker size)
		90 x 130 = 1:1.444 (nice looking scalable)
		89 x 137 = 1:1.539 (measured real card)
		57 x 89 = 1:1.561 (official bridge size)
	*/

	// Card padding is 10% of card height/width

	if ThePreferences.FixedCards {
		CardWidth = ThePreferences.FixedCardWidth
		PilePaddingX = CardWidth / 10
		CardHeight = ThePreferences.FixedCardHeight
		PilePaddingY = CardHeight / 10
		cardsWidth := PilePaddingX + CardWidth*(maxX+2)
		LeftMargin = (b.WindowWidth - cardsWidth) / 2
	} else {
		slotWidth := float64(b.WindowWidth) / float64(maxX+2)
		PilePaddingX = int(slotWidth / 10)
		CardWidth = int(slotWidth) - PilePaddingX
		slotHeight := slotWidth * ThePreferences.CardRatio
		PilePaddingY = int(slotHeight / 10)
		CardHeight = int(slotHeight) - PilePaddingY
		LeftMargin = (CardWidth / 2) + PilePaddingX
	}
	CardCornerRadius = float64(CardWidth) / 10.0 // same as lsol
	TopMargin = 48 + CardHeight/3

	if DebugMode {
		if CardWidth != OldWidth || CardHeight != OldHeight {
			println("ScaleCards did something")
		} else {
			println("ScaleCards did nothing")
		}
	}
	return CardWidth != OldWidth || CardHeight != OldHeight
}

func (b *Baize) PercentComplete() int {
	var pairs, unsorted, percent int
	for _, p := range b.piles {
		if p.Len() > 1 {
			pairs += p.Len() - 1
		}
		unsorted += p.vtable.UnsortedPairs()
	}
	// TheUI.SetMiddle(fmt.Sprintf("%d/%d", pairs-unsorted, pairs))
	percent = (int)(100.0 - util.MapValue(float64(unsorted), 0, float64(pairs), 0.0, 100.0))
	return percent
}

func (b *Baize) Recycles() int {
	return b.recycles
}

func (b *Baize) SetRecycles(recycles int) {
	b.recycles = recycles
	b.setFlag(dirtyPileBackgrounds) // recreate Stock placeholder
}

func (b *Baize) UpdateToolbar() {
	TheUI.EnableWidget("toolbarUndo", len(b.undoStack) > 1)
	TheUI.EnableWidget("toolbarCollect", b.fmoves > 0)
}

func (b *Baize) UpdateStatusbar() {
	if b.script.Stock().Hidden() {
		TheUI.SetStock(-1)
	} else {
		TheUI.SetStock(b.script.Stock().Len())
	}
	if b.script.Waste() == nil {
		TheUI.SetWaste(-1) // previous variant may have had a waste, and this one does not
	} else {
		TheUI.SetWaste(b.script.Waste().Len())
	}
	// if DebugMode {
	// 	TheUI.SetMiddle(fmt.Sprintf("MOVES: %d,%d", b.moves, b.fmoves))
	// }
	TheUI.SetMiddle(fmt.Sprintf("MOVES: %d", len(b.undoStack)-1))
	TheUI.SetPercent(b.PercentComplete())
}

func (b *Baize) Conformant() bool {
	for _, p := range b.piles {
		if !p.vtable.Conformant() {
			return false
		}
	}
	return true
}

func (b *Baize) Complete() bool {
	for _, p := range b.piles {
		if !p.vtable.Complete() {
			return false
		}
	}
	return true
}

// Layout implements ebiten.Game's Layout.
func (b *Baize) Layout(outsideWidth, outsideHeight int) (int, int) {

	if outsideWidth == 0 || outsideHeight == 0 {
		println("Baize.Layout called with zero dimension")
		return outsideWidth, outsideHeight
	}

	if DebugMode && (outsideWidth != b.WindowWidth || outsideHeight != b.WindowHeight) {
		println("Window resize to", outsideWidth, outsideHeight)
	}

	if outsideWidth != b.WindowWidth {
		b.setFlag(dirtyWindowSize | dirtyCardSizes | dirtyPileBackgrounds | dirtyPilePositions | dirtyCardPositions)
		b.WindowWidth = outsideWidth
	}
	if outsideHeight != b.WindowHeight {
		b.setFlag(dirtyWindowSize | dirtyCardPositions)
		b.WindowHeight = outsideHeight
	}

	if b.dirtyFlags != 0 {
		if b.flagSet(dirtyCardSizes) {
			if b.ScaleCards() {
				CreateCardImages()
				b.setFlag(dirtyPilePositions | dirtyPileBackgrounds)
			}
			b.clearFlag(dirtyCardSizes)
		}
		if b.flagSet(dirtyCardImages) {
			CreateCardImages()
			b.clearFlag(dirtyCardImages)
		}
		if b.flagSet(dirtyPilePositions) {
			for _, p := range b.piles {
				p.SetBaizePos(image.Point{
					X: LeftMargin + (p.Slot().X * (CardWidth + PilePaddingX)),
					Y: TopMargin + (p.Slot().Y * (CardHeight + PilePaddingY)),
				})
			}
			b.clearFlag(dirtyPilePositions)
		}
		if b.flagSet(dirtyPileBackgrounds) {
			if !(CardWidth == 0 || CardHeight == 0) {
				for _, p := range b.piles {
					if !p.Hidden() {
						p.img = p.vtable.Placeholder()
					}
				}
			}
			b.clearFlag(dirtyPileBackgrounds)
		}
		if b.flagSet(dirtyWindowSize) {
			TheUI.Layout(outsideWidth, outsideHeight)
			b.clearFlag(dirtyWindowSize)
		}
		if b.flagSet(dirtyCardPositions) {
			for _, p := range b.piles {
				p.Scrunch()
			}
			b.clearFlag(dirtyCardPositions)
		}
	}

	return outsideWidth, outsideHeight
}

// Update the baize state (transitions, user input)
func (b *Baize) Update() error {

	if b.stroke == nil {
		input.StartStroke(b) // this will set b.stroke when "start" received
	} else {
		b.stroke.Update()
		if b.stroke.IsReleased() || b.stroke.IsCancelled() {
			b.stroke = nil
		}
	}

	for _, p := range b.piles {
		p.Update()
	}

	for k := ebiten.Key(0); k <= ebiten.KeyMax; k++ {
		if inpututil.IsKeyJustReleased(k) {
			Execute(k)
		}
	}

	TheUI.Update()

	return nil
}

// Draw renders the baize into the screen
func (b *Baize) Draw(screen *ebiten.Image) {

	screen.Fill(ExtendedColors[ThePreferences.BaizeColor])

	for _, p := range b.piles {
		p.Draw(screen)
		// for _, c := range p.cards {
		// 	c.Draw(screen)
		// }
	}
	for _, p := range b.piles {
		p.DrawStaticCards(screen)
	}
	for _, p := range b.piles {
		p.DrawTransitioningCards(screen)
	}
	for _, p := range b.piles {
		p.DrawFlippingCards(screen)
	}
	for _, p := range b.piles {
		p.DrawDraggingCards(screen)
	}

	TheUI.Draw(screen)
	// if DebugMode {
	// var ms runtime.MemStats
	// runtime.ReadMemStats(&ms)
	// ebitenutil.DebugPrint(screen, fmt.Sprintf("FPS %v, Alloc %v, NumGC %v", ebiten.CurrentTPS(), ms.Alloc, ms.NumGC))
	// ebitenutil.DebugPrint(screen, fmt.Sprintf("%v %v", b.bookmark, len(b.undoStack)))
	// bounds := screen.Bounds()
	// ebitenutil.DebugPrint(screen, bounds.String())
	// }
}
