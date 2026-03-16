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

1. **Start the Pup** → Click "Enabled" in MENU to start the pup services
2. **View Metrics** → You'll see: `[🔒 Hidden - Check 'Click to Reveal Mnemonic' in Wallet Security settings to view]`
3. **Click Reveal Checkbox** → Go to **Settings → Wallet Security → Check "🔓 Click to Reveal Mnemonic"**
4. **Return to Metrics** → The actual mnemonic words will now be visible
5. **Save Your Mnemonic** → Copy and store it securely offline
6. **One-Time Only** → After you view it once, it permanently shows: `[Mnemonic was displayed and should have been saved]`

> **Note:** The reveal checkbox is **separate from** the main "Enabled" toggle that controls the entire pup. You must start the pup first, then use the reveal checkbox to see the mnemonic.

### Security Features

- 🔒 **Hidden by default** - Mnemonic starts masked, you must check the reveal box
- 🔓 **Reveal checkbox** - Dedicated checkbox in Wallet Security settings
- ⚠️ **One-time display** - Can only be viewed during first initialization
- 💾 **Never persisted** - Stored in temporary file, deleted after first display
- ✅ **Independent control** - Separate from pup enable/disable

**Important: After enabling the pup, go to Settings → Wallet Security → Check the reveal box to see your mnemonic, then save it securely!**
