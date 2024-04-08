package helpers

import (
	"regexp"
	"strconv"
	"strings"
)

// Offset converts a string offset value to an integer.
// If the conversion fails, it returns 0.
func Offset(offset string) int {
	offsetInt, err := strconv.Atoi(offset)
	if err != nil {
		offsetInt = 0
	}
	return offsetInt
}

// Limit converts a string representation of a limit to an integer.
// If the conversion fails, it returns a default limit of 25.
func Limit(limit string) int {
	limitInt, err := strconv.Atoi(limit)
	if err != nil {
		limitInt = 25
	}
	return limitInt
}

// SortOrder returns a string representing the sort order for a database query.
// It concatenates the table name, the snake-cased sort field, and the snake-cased order field.
// The resulting string is suitable for use in an ORDER BY clause of a SQL query.
// The table parameter specifies the name of the table.
// The sort parameter specifies the field to sort by.
// The order parameter specifies the sort order, which can be "ASC" for ascending or "DESC" for descending.
func SortOrder(table, sort, order string) string {
	return table + "." + ToSnakeCase(sort) + " " + ToSnakeCase(order)
}

// ToSnakeCase converts a string to snake case.
// It replaces all capital letters with an underscore followed by the lowercase letter.
// For example, "ToSnakeCase" becomes "to_snake_case".
func ToSnakeCase(str string) string {
	matchFirstCap := regexp.MustCompile("(.)([A-Z][a-z]+)")
	matchAllCap := regexp.MustCompile("([a-z0-9])([A-Z])")

	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")

	return strings.ToLower(snake)
}
