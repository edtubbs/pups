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

1. **Graphical Display (Metrics)**: The mnemonic appears as a metric in your Dogebox dashboard
2. **Logger Output**: Shows notification that mnemonic was generated (but NOT the actual mnemonic for security)

### How to Reveal Your Mnemonic

#### The mnemonic is **HIDDEN by default** for security. To reveal it:

1. **View Metrics Dashboard** → You'll see: `[Hidden - Enable 'Show Mnemonic' in settings to reveal]`
2. **Click the "Reveal Button"** → Go to **Settings → Display Settings → Enable "🔓 Reveal Wallet Mnemonic" toggle**
3. **Return to Metrics** → The actual mnemonic words will now be visible
4. **Save Your Mnemonic** → Copy and store it securely offline
5. **Hide It Again** → Disable the toggle in Settings to hide the mnemonic
6. **One-Time Only** → After you view it once, it becomes permanently hidden with message: `[Mnemonic was displayed and should have been saved]`

### Security Features

- 🔒 **Hidden by default** - Mnemonic starts masked, you must use reveal toggle
- 🔓 **Reveal button** - The toggle in Settings acts as your reveal button
- ⚠️ **One-time display** - Can only be viewed during first initialization
- 💾 **Never persisted** - Not saved to disk or logged for security
- 🔄 **Reversible hide** - Can disable toggle to hide it while still available

**Important: The toggle in Settings IS your reveal button. Enable it to see the mnemonic, save it securely, then disable the toggle to hide it again!**
