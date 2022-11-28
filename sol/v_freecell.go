package sol

//lint:file-ignore ST1005 Error messages are toasted, so need to be capitalized

import (
	"image"
)

type Freecell struct {
	ScriptBase
	wikipedia string
}

func (fc *Freecell) BuildPiles() {

	fc.stock = NewStock(image.Point{-5, -5}, FAN_NONE, 1, 4, nil, 0)

	fc.cells = nil
	for x := 0; x < 4; x++ {
		fc.cells = append(fc.cells, NewCell(image.Point{x, 0}))
	}

	fc.foundations = nil
	for x := 4; x < 8; x++ {
		f := NewFoundation(image.Point{x, 0})
		fc.foundations = append(fc.foundations, f)
		f.SetLabel("A")
	}

	fc.tableaux = nil
	for x := 0; x < 8; x++ {
		t := NewTableau(image.Point{x, 1}, FAN_DOWN, MOVE_ONE_PLUS)
		fc.tableaux = append(fc.tableaux, t)
	}

	fc.discards = nil
	fc.reserves = nil
	fc.waste = nil
}

func (fc *Freecell) StartGame() {
	for i := 0; i < 4; i++ {
		pile := fc.tableaux[i]
		for j := 0; j < 7; j++ {
			MoveCard(fc.stock, pile)
		}
	}
	for i := 4; i < 8; i++ {
		pile := fc.tableaux[i]
		for j := 0; j < 6; j++ {
			MoveCard(fc.stock, pile)
		}
	}
	if fc.stock.Len() > 0 {
		println("*** still", fc.stock.Len(), "cards in Stock")
	}
}

func (*Freecell) AfterMove() {
}

func (*Freecell) TailMoveError(tail []*Card) (bool, error) {
	var pile *Pile = tail[0].Owner()
	switch (pile).category {
	case "Tableau":
		for _, pair := range NewCardPairs(tail) {
			if ok, err := pair.Compare_DownAltColor(); !ok {
				return false, err
			}
		}
	}
	return true, nil
}

func (*Freecell) TailAppendError(dst *Pile, tail []*Card) (bool, error) {
	// why the pretty asterisks? google method pointer receivers in interfaces; *Tableau is a different type to Tableau
	switch (dst).category {
	case "Foundation":
		if dst.Empty() {
			return Compare_Empty(dst, tail[0])
		} else {
			return CardPair{dst.Peek(), tail[0]}.Compare_UpSuit()
		}
	case "Tableau":
		if dst.Empty() {
			return Compare_Empty(dst, tail[0])
		} else {
			return CardPair{dst.Peek(), tail[0]}.Compare_DownAltColor()
		}
	}
	return true, nil
}

func (*Freecell) UnsortedPairs(pile *Pile) int {
	return UnsortedPairs(pile, CardPair.Compare_DownAltColor)
}

func (*Freecell) TailTapped(tail []*Card) {
	tail[0].Owner().vtable.TailTapped(tail)
}

func (*Freecell) PileTapped(*Pile) {}

func (fc *Freecell) Wikipedia() string {
	return fc.wikipedia
}

func (*Freecell) CardColors() int {
	return 2
}
