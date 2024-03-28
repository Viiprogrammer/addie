package broker

import (
	"fmt"

	"github.com/gofiber/fiber/v2"
)

func (m *Broker) ProxyOnConnect(c *fiber.Ctx) (e error) {
	fmt.Println(string(c.Body()))
	fmt.Fprint(c, "{\"result\": {\"user\": \"56\"}}")
	return c.SendStatus(fiber.StatusOK)
}
