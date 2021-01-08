package controller

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

// HomePage func
func HomePage(c *gin.Context) {
	fmt.Println("Endpoint Hit: homePage")
	var homepage string
	homepage = "Welcome to the HomePage!"
	c.JSON(200, homepage)
}
