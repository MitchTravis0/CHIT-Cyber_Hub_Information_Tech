package totp

// Every sentence a user can see, in one place, so the tests can assert on the
// same string the UI shows. None of them ever names or echoes a secret, a
// passphrase or a code.
const (
	msgNoVault = "There is no vault on this machine yet. Create one, or import a vault file a colleague exported."

	msgVaultExists = "There is already a vault on this machine. Unlock it, or export it somewhere safe before starting again."

	msgLocked = "The vault is locked. Type the passphrase to open it."

	msgAutoLocked = "The vault locked itself after 15 minutes. Type the passphrase again."

	msgWrongPassphrase = "That passphrase did not open the vault. Check the caps lock key, and remember it is the vault's passphrase, not your Windows one. If you are sure the passphrase is right, the file has been changed since CHIT wrote it."

	msgTampered = "This vault file has been edited and cannot be trusted. Use a copy that came straight from CHIT."

	msgNewerVault = "This vault was written by a newer version of CHIT and could not be read. Update CHIT, or export it again from the machine that wrote it."

	msgPassphraseShort = "A vault passphrase has to be at least 12 characters. Use the Password Generator tool if you need one."

	msgPassphraseLong = "That passphrase is longer than 1024 characters, which is longer than anything you will type twice."

	msgNothingToAdd = "Paste the otpauth:// link, or type the secret with an issuer and a label."

	msgNotOtpauth = "That is not an otpauth link. It has to start with otpauth://totp/ and it is usually behind a \"can't scan the code?\" link next to the QR code."

	msgHOTP = "That is a counter-based (HOTP) code, which CHIT does not handle. Only time-based codes are supported."

	msgNoSecretInURI = "That link has no secret in it, so there is nothing to save. Copy the whole link, including everything after the question mark."

	msgSecretFormat = "That secret is not in the right format. A two factor secret is letters A to Z and digits 2 to 7, usually in groups of four."

	msgSecretShort = "That secret is too short to be a real one. A two factor secret is normally 16 or 32 letters and numbers."

	msgSecretLong = "That secret is longer than any real one. Check you have pasted the secret and not the whole page."

	msgNoName = "Give the account an issuer or a label, so you can tell it apart from the others."

	msgDuplicate = "%s %s is already in this vault."

	msgTooManyAccounts = "This vault already holds 200 accounts, which is as many as CHIT keeps in one file. Make a second vault for another site."

	msgDigits = "That account asks for %s digit codes. CHIT handles 6, 7 and 8."

	msgPeriod = "That account asks for a %s second code. CHIT handles 15 to 300 seconds, and almost everything uses 30."

	msgAlgorithm = "That account uses %s. CHIT handles SHA1, SHA256 and SHA512."

	msgNotFound = "That account is not in this vault. It may have been removed already."

	msgNotAVault = "That file is not a CHIT vault. Export one from this tool to see the format."

	msgImportWrongPassphrase = "That passphrase did not open that vault file. It is the passphrase the file was exported with, which may not be the one for the vault on this machine."

	msgImportLocked = "Unlock the vault on this machine first, so the imported accounts have somewhere to go."

	msgImportTooBig = "That file is larger than 1 MB. A vault file is a few kilobytes, so this is probably not one."

	msgSaveFailed = "The vault could not be saved. Check that the CHIT data folder is still there. Nothing was changed."

	msgReadFailed = "The vault file could not be read. Check that the CHIT data folder is still there."

	msgNoRandom = "This computer would not give CHIT the random numbers a new vault needs. Try again, and restart the machine if it keeps happening."
)
