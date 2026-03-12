<div align="center">
  <img src="../docs/img/dogebox-logo.png" alt="Dogebox Logo"/>
  <p>Libdogecoin SPV</p>
</div>

> [!CAUTION]  
> This pup does not have a stable release yet.

This pup will install [Libdogecoin SPV](https://github.com/dogecoinfoundation/libdogecoin) as a pup on your node.

It will generate a new wallet and start block sync from the last checkpoint.

## ⚠️ Important: One-Time Mnemonic Display

**On first initialization**, this pup will generate your wallet's mnemonic phrase. You must reveal it **ONCE** to save it:

### How to Reveal Your Mnemonic

#### The mnemonic is **HIDDEN by default** for security. To reveal it:

1. **View Metrics** → You'll see: `[🔒 Hidden - Check 'Click to Reveal Mnemonic' in Wallet Security settings to view]`
2. **Click Reveal Checkbox** → Go to **Settings → Wallet Security → Check "🔓 Click to Reveal Mnemonic"**
3. **Return to Metrics** → The actual mnemonic words will now be visible
4. **Save Your Mnemonic** → Copy and store it securely offline
5. **One-Time Only** → After you view it once, it permanently shows: `[Mnemonic was displayed and should have been saved]`

> **Note:** The reveal checkbox is **separate from** the main "Enabled" toggle that controls the entire pup.

### Security Features

- 🔒 **Hidden by default** - Mnemonic starts masked, you must check the reveal box
- 🔓 **Reveal checkbox** - Dedicated checkbox in Wallet Security settings
- ⚠️ **One-time display** - Can only be viewed during first initialization
- 💾 **Never persisted** - Not saved to disk or logged for security
- ✅ **Independent control** - Separate from pup enable/disable

**Important: Go to Settings → Wallet Security → Check the reveal box to see your mnemonic, then save it securely!**
