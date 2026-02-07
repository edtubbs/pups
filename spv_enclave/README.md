<div align="center">
  <img src="../docs/img/dogebox-logo.png" alt="Dogebox Logo"/>
  <p>Libdogecoin SPV</p>
</div>

> [!CAUTION]  
> This pup does not have a stable release yet.

This pup will install [Libdogecoin SPV](https://github.com/dogecoinfoundation/libdogecoin) as a pup on your node.

It will generate a new wallet and start block sync from the last checkpoint.

## ⚠️ Important: One-Time Mnemonic Display

**On first initialization**, this pup will display your wallet's mnemonic phrase **ONCE** in two ways:

1. **Graphical Display (Metrics)**: The mnemonic appears as a metric in your Dogebox dashboard
2. **Logger Output**: The mnemonic is also displayed in the logger output

- ✅ The mnemonic is displayed clearly with warning messages
- ✅ This is your **ONLY** opportunity to see and save the mnemonic
- ✅ The mnemonic is **NOT** saved to disk for security reasons
- ✅ After being displayed once, the metric will show "[Mnemonic was displayed and should have been saved]"
- ✅ If you miss it, you will need to recreate the wallet

**Save your mnemonic phrase immediately when you see it!**
