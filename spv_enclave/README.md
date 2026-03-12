<div align="center">
  <img src="../docs/img/dogebox-logo.png" alt="Dogebox Logo"/>
  <p>Libdogecoin SPV</p>
</div>

> [!CAUTION]  
> This pup does not have a stable release yet.

This pup will install [Libdogecoin SPV](https://github.com/dogecoinfoundation/libdogecoin) as a pup on your node.

It will generate a new wallet and start block sync from the last checkpoint.

## ⚠️ Important: One-Time Mnemonic Display

**On first initialization**, this pup will display your wallet's mnemonic phrase **ONCE** via the Metrics dashboard:

1. **Graphical Display (Metrics)**: The mnemonic appears as a metric in your Dogebox dashboard
2. **Logger Output**: Shows notification that mnemonic was generated (but NOT the actual mnemonic for security)

### Mnemonic Display Settings

- 👁️ **Default State**: The mnemonic is **visible by default** on first initialization
- 🔒 **Hide Option**: After saving your mnemonic, you can disable **"Show Mnemonic in Metrics"** in **Settings → Display Settings** to hide it from view
- ⚠️ **One-Time Availability**: The mnemonic is only available during the first initialization
- 💾 **Save Immediately**: When you see the mnemonic in metrics, save it immediately in a secure location

### Security Features

- ✅ The mnemonic is **visible on first initialization** so you don't miss it
- ✅ You can **hide it after saving** using the toggle in Settings
- ✅ The mnemonic is **NOT** saved to disk or logged for security reasons
- ✅ After being displayed once, the metric will show "[Mnemonic was displayed and should have been saved]"
- ✅ If you miss it, you will need to recreate the wallet

**Important: The mnemonic appears in the Metrics dashboard immediately on first startup. Save it securely, then disable the toggle in Settings to hide it!**
