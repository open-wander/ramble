package controller

import (
	"fmt"

	"github.com/gin-gonic/gin"
)

func NotImplemented(c *gin.Context) {
	fmt.Println("Endpoint Hit: NotImplemented")
	notimplemented := "Endpoint Not implemented yet"
	c.JSON(200, notimplemented)
}

func AuthNotImplemented(c *gin.Context) {
	fmt.Println("Endpoint Hit: AuthNotImplemented")
	authnotimplemented := "Auth Endpoint Not implemented yet"
	c.JSON(200, authnotimplemented)
}
