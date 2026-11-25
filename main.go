
package main

import (
    "fmt"

    "fyne.io/fyne/v2"
    "fyne.io/fyne/v2/app"
    "fyne.io/fyne/v2/driver/desktop"

    "heckel.io/ntfy/v2/client"

    _ "modernc.org/sqlite"
)

func main() {
    fmt.Println("Starting API verification...")

    // Verify Fyne application.
    a := app.New()

    // Verify ntfy configuration.
    config := client.NewConfig()

    // Verify ntfy client.
    ntfyClient := client.New(config)
    _ = ntfyClient

    // Verify notification API.
    notification := fyne.NewNotification(
        "ntfy Desktop",
        "API verification successful",
    )
    a.SendNotification(notification)

    // Verify desktop tray interface.
    if desk, ok := a.(desktop.App); ok {
        fmt.Println("Desktop tray interface available")
        _ = desk
    }

    fmt.Println("Fyne, ntfy and SQLite imports successful")
}
