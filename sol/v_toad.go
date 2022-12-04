package sol

//lint:file-ignore ST1005 Error messages are toasted, so need to be capitalized

import (
	"errors"
	"image"

	"oddstream.games/gosol/util"
)

type Toad struct {
	ScriptBase
	wikipedia string
}

func (t *Toad) BuildPiles() {

	t.stock = NewStock(image.Point{0, 0}, FAN_NONE, 2, 4, nil, 0)
	t.waste = NewWaste(image.Point{1, 0}, FAN_RIGHT3)

	t.reserves = nil
	t.reserves = append(t.reserves, NewReserve(image.Point{3, 0}, FAN_RIGHT))

	t.foundations = nil
	for x := 0; x < 8; x++ {
		t.foundations = append(t.foundations, NewFoundation(image.Point{x, 1}))
	}

	t.tableaux = nil
	for x := 0; x < 8; x++ {
		// When moving tableau piles, you must either move the whole pile or only the top card.
		t.tableaux = append(t.tableaux, NewTableau(image.Point{x, 2}, FAN_DOWN, MOVE_ONE_OR_ALL))
	}
}

func (t *Toad) StartGame() {

	TheBaize.SetRecycles(1)

	for n := 0; n < 20; n++ {
		MoveCard(t.stock, t.reserves[0])
		t.reserves[0].Peek().FlipDown()
	}
	t.reserves[0].Peek().FlipUp()

	for _, pile := range t.tableaux {
		MoveCard(t.stock, pile)
	}
	// One card is dealt onto the first foundation. This rank will be used as a base for the other foundations.
	c := MoveCard(t.stock, t.foundations[0])
	for _, pile := range t.foundations {
		pile.SetLabel(util.OrdinalToShortString(c.Ordinal()))
	}
	MoveCard(t.stock, t.waste)
}

func (t *Toad) AfterMove() {
	// Empty spaces are filled automatically from the reserve.
	for _, p := range t.tableaux {
		if p.Empty() {
			MoveCard(t.reserves[0], p)
		}
	}
	if t.waste.Len() == 0 && t.stock.Len() != 0 {
		MoveCard(t.stock, t.waste)
	}

}

func (*Toad) TailMoveError(tail []*Card) (bool, error) {
	return true, nil
}

func (t *Toad) TailAppendError(dst *Pile, tail []*Card) (bool, error) {
	// why the pretty asterisks? google method pointer receivers in interfaces; *Tableau is a different type to Tableau
	switch dst.category {
	case "Foundation":
		if dst.Empty() {
			return Compare_Empty(dst, tail[0])
		} else {
			return CardPair{dst.Peek(), tail[0]}.Compare_UpSuitWrap()
		}
	case "Tableau":
		if dst.Empty() {
			// Once the reserve is empty, spaces in the tableau can be filled with a card from the Deck [Stock/Waste], but NOT from another tableau pile.
			// pointless rule, since tableuax move rule is MOVE_ONE_OR_ALL
			if tail[0].owner != t.waste {
				return false, errors.New("Empty tableaux must be filled with cards from the waste")
			}
		} else {
			return CardPair{dst.Peek(), tail[0]}.Compare_DownSuitWrap()
		}
	}
	return true, nil
}

func (*Toad) UnsortedPairs(pile *Pile) int {
	return UnsortedPairs(pile, CardPair.Compare_DownSuitWrap)
}

func (t *Toad) TailTapped(tail []*Card) {
	var pile *Pile = tail[0].Owner()
	if pile == t.stock && len(tail) == 1 {
		c := pile.Pop()
		t.waste.Push(c)
	} else {
		pile.vtable.TailTapped(tail)
	}
}

func (t *Toad) PileTapped(pile *Pile) {
	if pile == t.stock {
		RecycleWasteToStock(t.waste, t.stock)
	}
}

func (t *Toad) Wikipedia() string {
	return t.wikipedia
}

func (t *Toad) CardColors() int {
	return 4
}
