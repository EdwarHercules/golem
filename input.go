package main

import "fmt"

func processOrder(status string, items []string,
  userAge int, isPremium bool, hasDiscount bool) string {
  result := ""
  if status == "active" {
    if userAge >= 18 {
      if len(items) > 0 {
        for _, item := range items {
          if item != "" {
            if isPremium {
              if hasDiscount {
                result += "PREMIUM_DISCOUNT:" + item + ";"
              } else {
                result += "PREMIUM:" + item + ";"
              }
            } else {
              if hasDiscount {
                result += "DISCOUNT:" + item + ";"
              } else {
                result += "NORMAL:" + item + ";"
              }
            }
          }
        }
      } else {
        result = "NO_ITEMS"
      }
    } else {
      result = "UNDERAGE"
    }
  } else if status == "pending" {
    result = "PENDING"
  } else if status == "cancelled" {
    result = "CANCELLED"
  } else {
    result = "UNKNOWN"
  }
  return result
}

func calculateTotal(prices []float64, tax float64,
  discount float64, currency string) float64 {
  total := 0.0
  for _, p := range prices {
    total += p
  }
  if discount > 0 {
    total = total - (total * discount / 100)
  }
  if tax > 0 {
    total = total + (total * tax / 100)
  }
  if currency == "USD" {
    return total
  } else if currency == "EUR" {
    return total * 0.92
  } else if currency == "GBP" {
    return total * 0.79
  }
  return total
}

func main() {
  items := []string{"laptop", "mouse", "keyboard"}
  result := processOrder("active", items, 25, true, true)
  fmt.Println(result)
  total := calculateTotal([]float64{100, 200, 300},
    15, 10, "USD")
  fmt.Printf("Total: %.2f\n", total)
}