package handlers

import "github.com/gin-gonic/gin"

func JSONOK(c *gin.Context, v any) {
	c.Header("Content-Type", "application/json; charset=utf-8")
	c.JSON(200, v)
}
func JSONBadRequest(c *gin.Context, msg string) {
	c.JSON(400, gin.H{"error": msg})
}
func JSONNotFound(c *gin.Context, msg string) {
	c.JSON(404, gin.H{"error": msg})
}
func JSONServerErr(c *gin.Context, msg string) {
	c.JSON(500, gin.H{"error": msg})
}
