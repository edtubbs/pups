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

- 🔒 **Default State**: The mnemonic is **hidden/masked** by default for security
- 👁️ **Show Mnemonic Toggle**: Go to **Settings → Display Settings** and enable **"Show Mnemonic in Metrics"** to reveal it
- ⚠️ **One-Time Availability**: The mnemonic is only available during the first initialization
- 💾 **Save Immediately**: Once you reveal and view the mnemonic, save it immediately in a secure location

### Security Features

- ✅ The mnemonic is **masked by default** - you must explicitly enable display
- ✅ The mnemonic is **NOT** saved to disk or logged for security reasons
- ✅ After being displayed once, the metric will show "[Mnemonic was displayed and should have been saved]"
- ✅ If you miss it, you will need to recreate the wallet

**To view your mnemonic: Enable "Show Mnemonic in Metrics" in Settings, then check the Metrics dashboard!**
