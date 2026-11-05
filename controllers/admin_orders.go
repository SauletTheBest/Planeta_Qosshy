package controllers

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"planeta_qosshy/database"
	"planeta_qosshy/middleware"
	"planeta_qosshy/models"
	"planeta_qosshy/shop"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// needsRefundSQL selects orders that have a payment the shop must give back.
const needsRefundSQL = "EXISTS (SELECT 1 FROM payments p WHERE p.order_id = orders.id AND p.status = ? AND p.refund_needed_at IS NOT NULL)"

// orderTab is one filter button above the orders table.
type orderTab struct {
	Key   string
	Label string
	Count int64
}

func AdminDashboard(c *gin.Context) {
	stats, err := shop.DashboardStats(database.DB, time.Now())
	if err != nil {
		adminFail(c, err)
		return
	}
	c.HTML(http.StatusOK, "admin_dashboard.html", gin.H{"s": stats, "flash": middleware.PopFlash(c)})
}

// AdminOrders: GET /admin/orders?status=paid&q=alice&page=2
func AdminOrders(c *gin.Context) {
	status := c.Query("status")
	search := strings.TrimSpace(c.Query("q"))
	page := newPage(c.Query("page"), "", 20, 20)

	query := database.DB.Model(&models.Order{})
	switch status {
	case "":
	case "refund":
		query = query.Where(needsRefundSQL, models.PaymentSucceeded)
	default:
		query = query.Where("orders.status = ?", status)
	}
	if search != "" {
		// "15" or "#15" is an order number, anything else is a customer's email or name.
		if id, err := strconv.ParseUint(strings.TrimPrefix(search, "#"), 10, 32); err == nil {
			query = query.Where("orders.id = ?", id)
		} else {
			p := likePattern(search)
			query = query.Where("EXISTS (SELECT 1 FROM users u WHERE u.id = orders.user_id AND (u.email ILIKE ? OR u.username ILIKE ?))", p, p)
		}
	}

	var total int64
	if err := query.Count(&total).Error; err != nil {
		adminFail(c, err)
		return
	}
	page.SetTotal(total)

	var orders []models.Order
	if err := query.Preload("User").Preload("Items").
		Preload("Payments", func(db *gorm.DB) *gorm.DB { return db.Order("id DESC") }).
		Order("orders.id DESC").Offset(page.Offset()).Limit(page.Size).
		Find(&orders).Error; err != nil {
		adminFail(c, err)
		return
	}

	tabs, err := orderTabs()
	if err != nil {
		adminFail(c, err)
		return
	}

	c.HTML(http.StatusOK, "ADMIN_orders.html", gin.H{
		"orders": orders, "tabs": tabs, "status": status, "q": search,
		"pager": page, "flash": middleware.PopFlash(c),
	})
}

// orderTabs counts orders per status with one GROUP BY instead of five COUNT queries.
func orderTabs() ([]orderTab, error) {
	var rows []struct {
		Status models.OrderStatus
		N      int64
	}
	if err := database.DB.Model(&models.Order{}).Select("status, count(*) AS n").
		Group("status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	counts := map[models.OrderStatus]int64{}
	var all int64
	for _, r := range rows {
		counts[r.Status] = r.N
		all += r.N
	}

	var refunds int64
	if err := database.DB.Model(&models.Order{}).Where(needsRefundSQL, models.PaymentSucceeded).
		Count(&refunds).Error; err != nil {
		return nil, err
	}

	tabs := []orderTab{{"", "Все", all}}
	for _, s := range []models.OrderStatus{models.OrderPaid, models.OrderPending, models.OrderShipped,
		models.OrderDelivered, models.OrderCancelled} {
		tabs = append(tabs, orderTab{string(s), s.Label(), counts[s]})
	}
	return append(tabs, orderTab{"refund", "Нужен возврат", refunds}), nil
}

// AdminOrder: GET /admin/orders/:id — everything about one order and the allowed actions.
func AdminOrder(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Заказ не найден"})
		return
	}
	var order models.Order
	err := database.DB.Preload("User").Preload("Items").
		Preload("Payments", func(db *gorm.DB) *gorm.DB { return db.Order("id DESC") }).
		Preload("Events", func(db *gorm.DB) *gorm.DB { return db.Order("id") }).
		Preload("Events.Actor").
		First(&order, id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Заказ не найден"})
		return
	}
	if err != nil {
		adminFail(c, err)
		return
	}
	c.HTML(http.StatusOK, "ADMIN_order.html", gin.H{
		"order": order,
		"next":  shop.NextStatuses(order.Status), // buttons come from the same table the rules use
		"flash": middleware.PopFlash(c),
	})
}

// AdminOrderStatus: POST /admin/orders/:id/status  to=shipped|delivered|cancelled [reason=...]
func AdminOrderStatus(c *gin.Context) {
	id, ok := parseID(c.Param("id"))
	if !ok {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Заказ не найден"})
		return
	}
	to := models.OrderStatus(c.PostForm("to"))
	adminID := c.GetUint("userID")
	back := fmt.Sprintf("/admin/orders/%d", id)
	done := func(kind, text string) {
		middleware.SetFlash(c, kind, text)
		c.Redirect(http.StatusSeeOther, back) // Post/Redirect/Get: F5 won't repeat the action
	}
	audit := middleware.Log().WithField("admin_id", adminID).WithField("order_id", id).WithField("to", to)

	if to != models.OrderCancelled {
		err := shop.ChangeStatus(database.DB, id, to, adminID)
		switch {
		case errors.Is(err, shop.ErrBadTransition):
			done("error", "Такой переход статуса недоступен. Возможно, заказ уже изменил кто-то другой.")
		case err != nil:
			audit.WithError(err).Error("admin: change order status")
			done("error", "Не удалось изменить статус.")
		default:
			audit.Info("admin: order status changed")
			done("success", fmt.Sprintf("Заказ №%d: %s.", id, strings.ToLower(to.Label())))
		}
		return
	}

	reason := strings.TrimSpace(c.PostForm("reason"))
	if reason == "" {
		done("error", "Укажите причину отмены: она останется в истории заказа.")
		return
	}
	paymentID, err := shop.CancelOrder(database.DB, id, adminID, reason, time.Now())
	switch {
	case errors.Is(err, shop.ErrBadTransition):
		done("error", "Этот заказ уже нельзя отменить.")
		return
	case err != nil:
		audit.WithError(err).Error("admin: cancel order")
		done("error", "Не удалось отменить заказ.")
		return
	}
	audit.WithField("reason", reason).Info("admin: order cancelled")
	if paymentID == 0 {
		done("success", fmt.Sprintf("Заказ №%d отменён, товар вернулся в продажу.", id))
		return
	}

	// The order was paid: give the money back right away.
	if err := refund(c, paymentID, adminID); err != nil {
		done("error", fmt.Sprintf("Заказ №%d отменён, но банк не принял возврат (%v). "+
			"Нажмите «Вернуть деньги», когда банк будет доступен.", id, err))
		return
	}
	done("success", fmt.Sprintf("Заказ №%d отменён, деньги возвращены покупателю.", id))
}

// AdminRefund: POST /admin/orders/:id/refund  payment_id=...  (retry from the order page)
func AdminRefund(c *gin.Context) {
	orderID, ok1 := parseID(c.Param("id"))
	paymentID, ok2 := parseID(c.PostForm("payment_id"))
	back := fmt.Sprintf("/admin/orders/%d", orderID)
	if !ok1 || !ok2 {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Платёж не найден"})
		return
	}
	// The payment must belong to the order in the URL: otherwise a form with someone
	// else's payment_id would refund a different order than the page showed.
	var p models.Payment
	if err := database.DB.Where("id = ? AND order_id = ?", paymentID, orderID).First(&p).Error; err != nil {
		c.HTML(http.StatusNotFound, "error.html", gin.H{"error": "Платёж не найден"})
		return
	}

	err := refund(c, p.ID, c.GetUint("userID"))
	switch {
	case errors.Is(err, shop.ErrNothingToRefund):
		middleware.SetFlash(c, "error", "Этот платёж не нужно возвращать.")
	case err != nil:
		middleware.SetFlash(c, "error", fmt.Sprintf("Банк не принял возврат: %v", err))
	default:
		middleware.SetFlash(c, "success", fmt.Sprintf("Деньги по платежу №%d возвращены.", p.ID))
	}
	c.Redirect(http.StatusSeeOther, back)
}

func refund(c *gin.Context, paymentID, adminID uint) error {
	// The bank gets 20 s at most; the admin's page must not hang forever.
	ctx, cancel := context.WithTimeout(c.Request.Context(), 20*time.Second)
	defer cancel()
	err := shop.RefundPayment(ctx, database.DB, paymentID, &adminID, time.Now())
	entry := middleware.Log().WithField("admin_id", adminID).WithField("payment_id", paymentID)
	if err != nil {
		entry.WithError(err).Warn("admin: refund failed")
	} else {
		entry.Info("admin: refund done")
	}
	return err
}

// adminFail logs the real error and shows a generic page: SQL details are not for the screen.
func adminFail(c *gin.Context, err error) {
	middleware.Log().WithError(err).WithField("path", c.Request.URL.Path).Error("admin page failed")
	c.HTML(http.StatusInternalServerError, "error.html", gin.H{"error": "Что-то пошло не так, подробности в логе сервера."})
}
