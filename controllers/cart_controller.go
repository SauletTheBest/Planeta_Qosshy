package controllers

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strconv"
	"time"

	"planeta_qosshy/database"
	"planeta_qosshy/middleware"
	"planeta_qosshy/models"
	"planeta_qosshy/shop"

	"github.com/gin-gonic/gin"
)

func CartPage(c *gin.Context) {
	lines, total, err := shop.LoadCart(database.DB, c.GetUint("userID"))
	if err != nil {
		log.Println("cart:", err)
		c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Не удалось открыть корзину"})
		return
	}
	c.HTML(http.StatusOK, "cart.html", gin.H{
		"lines":       lines,
		"total":       total,
		"reserveMins": int(shop.ReservationTTL.Minutes()),
		"maxQty":      shop.MaxCartQuantity,
		"flash":       middleware.PopFlash(c),
	})
}

func backToCart(c *gin.Context, kind, text string) {
	middleware.SetFlash(c, kind, text)
	c.Redirect(http.StatusSeeOther, "/cart")
}

// CartAdd: POST /cart/add  clothes_id, size, quantity
func CartAdd(c *gin.Context) {
	clothesID, ok := parseID(c.PostForm("clothes_id"))
	if !ok {
		backToCart(c, "error", "Товар не найден.")
		return
	}
	var item models.Clothes
	if err := database.DB.Preload("Variants").First(&item, clothesID).Error; err != nil {
		backToCart(c, "error", "Товар не найден.")
		return
	}
	size, ok := sizeToBuy(item, c.PostForm("size")) // the same rule as "buy now"
	if !ok {
		backToCart(c, "error", fmt.Sprintf("Выберите размер товара «%s».", item.Title))
		return
	}
	qty, _ := strconv.Atoi(c.PostForm("quantity"))

	err := shop.AddToCart(database.DB, c.GetUint("userID"), item.ID, size, qty)
	switch {
	case errors.Is(err, shop.ErrNoSuchSize):
		backToCart(c, "error", "У этого товара нет такого размера.")
	case errors.Is(err, shop.ErrOutOfStock):
		backToCart(c, "error", fmt.Sprintf("«%s» размера %s закончился.", item.Title, size))
	case errors.Is(err, shop.ErrCartFull):
		backToCart(c, "error", fmt.Sprintf("В корзине уже %d разных товаров — оформите часть заказа.", shop.MaxCartLines))
	case err != nil:
		log.Println("cart add:", err)
		backToCart(c, "error", "Не удалось добавить в корзину.")
	default:
		backToCart(c, "success", fmt.Sprintf("«%s» (%s) в корзине.", item.Title, size))
	}
}

// CartUpdate: POST /cart/update  variant_id, quantity (0 = remove)
func CartUpdate(c *gin.Context) {
	variantID, ok := parseID(c.PostForm("variant_id"))
	qty, err := strconv.Atoi(c.PostForm("quantity"))
	if !ok || err != nil {
		backToCart(c, "error", "Неверное количество.")
		return
	}
	// WHERE user_id = me: the variant_id from the form can't touch someone else's cart.
	if err := shop.SetCartQuantity(database.DB, c.GetUint("userID"), variantID, qty); err != nil {
		log.Println("cart update:", err)
		backToCart(c, "error", "Не удалось изменить корзину.")
		return
	}
	c.Redirect(http.StatusSeeOther, "/cart")
}

// CartCheckout: POST /cart/checkout — one order for the whole cart, then off to the bank.
func CartCheckout(c *gin.Context) {
	order, err := shop.CheckoutCart(database.DB, c.GetUint("userID"), time.Now())
	var oos *shop.OutOfStockError
	switch {
	case errors.As(err, &oos):
		// The whole order was rolled back: nothing is reserved, the cart is unchanged.
		msg := fmt.Sprintf("«%s» (%s) закончился — уберите его из корзины.", oos.Title, oos.Size)
		if oos.Left > 0 {
			msg = fmt.Sprintf("«%s» (%s): осталось только %d шт. — уменьшите количество.", oos.Title, oos.Size, oos.Left)
		}
		backToCart(c, "error", msg)
		return
	case errors.Is(err, shop.ErrEmptyCart):
		backToCart(c, "info", "Корзина пуста.")
		return
	case err != nil:
		log.Println("cart checkout:", err)
		backToCart(c, "error", "Не удалось оформить заказ, попробуйте ещё раз.")
		return
	}
	redirectToBank(c, order.ID)
}
