from selenium import webdriver
from selenium.webdriver.common.by import By
from selenium.webdriver.common.keys import Keys
import time


driver = webdriver.Edge()


BASE_URL = "http://localhost:8080"

try:
    print("Testing User Login...")
    driver.get(f"{BASE_URL}/auth/login")
    time.sleep(2)


    driver.find_element(By.NAME, "email").send_keys("admin@example.com")
    driver.find_element(By.NAME, "password").send_keys("admin")
    driver.find_element(By.ID, "login_button").click()

    driver.get(f"{BASE_URL}/admin/clothes/new")
    time.sleep(2)

    assert "Add New Clothes" in driver.page_source

    print("All E2E tests passed successfully!")

except Exception as e:
    print(f"Test failed: {e}")

finally:
    driver.quit()
