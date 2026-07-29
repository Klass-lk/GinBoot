package ginboot_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/klass-lk/ginboot"
	"github.com/klass-lk/ginboot/config"
	memory "github.com/klass-lk/ginboot/db/inmemory"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// DTOs for integration testing
type UserDTO struct {
	ID    string `json:"id" ginboot:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func (u UserDTO) GetTableName() string {
	return "users"
}

type OrderDTO struct {
	ID        string  `json:"id" ginboot:"id"`
	UserID    string  `json:"user_id"`
	UserName  string  `json:"user_name"`
	UserEmail string  `json:"user_email"`
	Amount    float64 `json:"amount"`
	Status    string  `json:"status"`
}

func (o OrderDTO) GetTableName() string {
	return "orders"
}

type NotificationDTO struct {
	ID      string `json:"id" ginboot:"id"`
	OrderID string `json:"order_id"`
	Message string `json:"message"`
}

func (n NotificationDTO) GetTableName() string {
	return "notifications"
}

type CreateOrderRequest struct {
	UserID string  `json:"user_id"`
	Amount float64 `json:"amount"`
}

// ----------------------------------------------------
// Service B: UserService & NotificationService Controller
// ----------------------------------------------------
type ServiceBController struct {
	userRepo         *memory.InMemoryRepository[UserDTO]
	notificationRepo *memory.InMemoryRepository[NotificationDTO]
	mu               sync.Mutex
	notifications    []NotificationDTO
}

func NewServiceBController(userRepo *memory.InMemoryRepository[UserDTO], notificationRepo *memory.InMemoryRepository[NotificationDTO]) *ServiceBController {
	return &ServiceBController{
		userRepo:         userRepo,
		notificationRepo: notificationRepo,
		notifications:    make([]NotificationDTO, 0),
	}
}

func (c *ServiceBController) Register(group *ginboot.ControllerGroup) {
	group.GET("/users/:id", c.GetUser)
	group.POST("/notifications", c.SendNotification)
}

func (c *ServiceBController) GetUser(ctx *ginboot.Context) (UserDTO, error) {
	id := ctx.Param("id")
	user, err := c.userRepo.FindById(id)
	if err != nil {
		return UserDTO{}, ginboot.NewApiError(http.StatusNotFound, "User not found")
	}
	return user, nil
}

func (c *ServiceBController) SendNotification(ctx *ginboot.Context, req NotificationDTO) (ginboot.EmptyResponse, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	req.ID = fmt.Sprintf("notif-%d", time.Now().UnixNano())
	_ = c.notificationRepo.Save(req)
	c.notifications = append(c.notifications, req)

	return ginboot.EmptyResponse{}, nil
}

// ----------------------------------------------------
// Service A: OrderService Controller
// ----------------------------------------------------
type OrderController struct {
	orderRepo *memory.InMemoryRepository[OrderDTO]
}

func NewOrderController(orderRepo *memory.InMemoryRepository[OrderDTO]) *OrderController {
	return &OrderController{
		orderRepo: orderRepo,
	}
}

func (c *OrderController) Register(group *ginboot.ControllerGroup) {
	group.POST("/orders", c.CreateOrder)
}

func (c *OrderController) CreateOrder(ctx *ginboot.Context, req CreateOrderRequest) (OrderDTO, error) {
	// 1. Synchronously call user-service on Service B to read user info (GET /users/:id)
	var user UserDTO
	err := ctx.CallServiceWithMethod("GET", "user-service", "/api/v1/users/"+req.UserID, nil, &user)
	if err != nil {
		return OrderDTO{}, ginboot.NewApiError(http.StatusBadRequest, fmt.Sprintf("Invalid user call failed: %v", err))
	}

	// 2. Perform DB write on local Service A
	order := OrderDTO{
		ID:        fmt.Sprintf("ord-%d", time.Now().UnixNano()),
		UserID:    user.ID,
		UserName:  user.Name,
		UserEmail: user.Email,
		Amount:    req.Amount,
		Status:    "CREATED",
	}

	if err := c.orderRepo.Save(order); err != nil {
		return OrderDTO{}, err
	}

	// 3. Perform fire-and-forget non-blocking async call to notification-service on Service B
	_ = ctx.CallServiceAsync("notification-service", "/api/v1/notifications", NotificationDTO{
		OrderID: order.ID,
		Message: fmt.Sprintf("Order %s created for %s", order.ID, user.Name),
	})

	return order, nil
}

// ----------------------------------------------------
// End-to-End Multi-Instance Service Communication Test
// ----------------------------------------------------
func TestServiceToServiceCommunication_EndToEnd(t *testing.T) {
	portB := 18082
	portA := 18081

	// Setup Repositories
	userRepoB := memory.NewInMemoryRepository[UserDTO]()
	notificationRepoB := memory.NewInMemoryRepository[NotificationDTO]()
	orderRepoA := memory.NewInMemoryRepository[OrderDTO]()

	// Seed User Data in Service B
	testUser := UserDTO{ID: "usr-99", Name: "Alice Smith", Email: "alice@example.com"}
	require.NoError(t, userRepoB.Save(testUser))

	// 1. Initialize Service B (Remote Service)
	serverB := ginboot.New()
	serverB.SetBasePath("/api/v1")
	controllerB := NewServiceBController(userRepoB, notificationRepoB)
	serverB.RegisterController("", controllerB)

	go func() {
		_ = serverB.Start(portB)
	}()

	// 2. Initialize Service A (Caller Service)
	serverA := ginboot.New()
	serverA.SetBasePath("/api/v1")

	// Configure Service A to route 'user-service' and 'notification-service' to Service B
	cfgA := &config.Config{
		Ginboot: config.GinbootRootConfig{
			Server: config.ServerConfig{Port: portA, BasePath: "/api/v1"},
			Services: map[string]config.ServiceTargetConfig{
				"user-service": {
					URL: fmt.Sprintf("http://localhost:%d", portB),
				},
				"notification-service": {
					URL: fmt.Sprintf("http://localhost:%d", portB),
				},
			},
		},
	}
	serverA.SetConfig(cfgA)

	controllerA := NewOrderController(orderRepoA)
	serverA.RegisterController("", controllerA)

	go func() {
		_ = serverA.Start(portA)
	}()

	// Wait briefly for both HTTP servers to start up
	time.Sleep(300 * time.Millisecond)

	// 3. Client calls Service A (POST http://localhost:18081/api/v1/orders)
	createReq := CreateOrderRequest{
		UserID: "usr-99",
		Amount: 149.99,
	}
	reqBytes, err := json.Marshal(createReq)
	require.NoError(t, err)

	resp, err := http.Post(fmt.Sprintf("http://localhost:%d/api/v1/orders", portA), "application/json", bytes.NewReader(reqBytes))
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)

	bodyBytes, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	var createdOrder OrderDTO
	err = json.Unmarshal(bodyBytes, &createdOrder)
	require.NoError(t, err)

	// Verify Service A successfully read user info from Service B via ctx.CallService
	assert.NotEmpty(t, createdOrder.ID)
	assert.Equal(t, "usr-99", createdOrder.UserID)
	assert.Equal(t, "Alice Smith", createdOrder.UserName)
	assert.Equal(t, "alice@example.com", createdOrder.UserEmail)
	assert.Equal(t, 149.99, createdOrder.Amount)
	assert.Equal(t, "CREATED", createdOrder.Status)

	// Verify local DB write on Service A succeeded
	savedOrder, err := orderRepoA.FindById(createdOrder.ID)
	require.NoError(t, err)
	assert.Equal(t, "Alice Smith", savedOrder.UserName)

	// 4. Verify Async Fire-and-Forget notification call reached Service B
	require.Eventually(t, func() bool {
		controllerB.mu.Lock()
		defer controllerB.mu.Unlock()
		return len(controllerB.notifications) > 0
	}, 3*time.Second, 100*time.Millisecond, "Expected async notification to be received by Service B")

	controllerB.mu.Lock()
	receivedNotif := controllerB.notifications[0]
	controllerB.mu.Unlock()

	assert.Equal(t, createdOrder.ID, receivedNotif.OrderID)
	assert.Contains(t, receivedNotif.Message, "Alice Smith")
}
