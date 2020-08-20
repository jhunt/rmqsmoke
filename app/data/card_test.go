package data_test

import (
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	"testing"

	"github.com/jhunt/rmqsmoke/data"
)

func TestData(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Data Test Suite")
}

var _ = Describe("Cards", func() {
	Context("with a small number of card slots", func() {
		var card *data.Card

		BeforeEach(func() {
			card = data.NewCard(5)
		})

		It("should handle empty cards", func () {
			Ω(card.Missing()).Should(Equal(5))
			Ω(card.Complete()).Should(BeFalse())
		})

		It("should handle a card with its initial slot filled", func() {
			card.Track(0)
			Ω(card.Missing()).Should(Equal(4))
			Ω(card.MissingValues()).Should(Equal([]int{
				1, 2, 3, 4,
			}))
			Ω(card.Complete()).Should(BeFalse())
		})

		It("should handle a card that is filled sequentially", func() {
			for i := 0; i < 5; i++ {
				card.Track(i)
			}
			Ω(card.Missing()).Should(Equal(0))
			Ω(card.MissingValues()).Should(Equal([]int{}))
			Ω(card.Complete()).Should(BeTrue())
		})
	})
})
