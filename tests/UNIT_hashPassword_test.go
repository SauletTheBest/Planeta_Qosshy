package tests

import (
	"planeta_qosshy/util"
	"testing"

	"github.com/stretchr/testify/assert"
	"golang.org/x/crypto/bcrypt"
)

func TestHashPassword(t *testing.T) {
	password := "securepassword123"

	hashedPassword := util.HashPassword(password)

	assert.NotEmpty(t, hashedPassword, "Hashed password should not be empty")

	err := bcrypt.CompareHashAndPassword([]byte(hashedPassword), []byte(password))
	assert.NoError(t, err, "Hashed password should match the original password")

	hashedPassword2 := util.HashPassword(password)
	assert.NotEqual(t, hashedPassword, hashedPassword2, "Two hashes of the same password should be different because salting")
}
