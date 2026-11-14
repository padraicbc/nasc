package app.nasc

import android.os.Build
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import java.io.File
import java.security.KeyStore
import java.security.SecureRandom
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/**
 * Supplies the 32-byte master key the Go core uses to encrypt saved SSH private keys.
 *
 * The master key is random and only ever stored wrapped (AES-GCM) by a non-exportable
 * Android Keystore key (hardware-backed — TEE or StrongBox — where the device has it).
 *
 * Two modes, told apart by which file exists:
 *  - PLAIN  (master.wrapped): wrapped by a Keystore key anyone running the app can use.
 *  - LOCKED (master.locked):  wrapped by a Keystore key that needs the user's biometric / device
 *    credential for *every* use. Without authenticating, the master key cannot be recovered —
 *    even by code running inside the app.
 *
 * If a Keystore key is gone we fail loudly instead of making a new master key, which would orphan
 * every saved SSH key. Adding a fingerprint does not invalidate the lock key (see lockKey()).
 */
object SecureKey {
    private const val PLAIN_ALIAS = "nasc_master_wrap"
    private const val LOCK_ALIAS = "nasc_master_lock"
    private const val PLAIN_FILE = "master.wrapped"
    private const val LOCK_FILE = "master.locked"

    private fun keyStore() = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }

    fun isLocked(dir: File) = File(dir, LOCK_FILE).exists()

    // ---- plain mode ------------------------------------------------------------------------

    fun plainKey(dir: File): ByteArray {
        check(!isLocked(dir)) { "App lock is on" }
        dir.mkdirs()
        val file = File(dir, PLAIN_FILE)
        if (!file.exists()) {
            val key = ByteArray(32).also { SecureRandom().nextBytes(it) }
            wrapPlain(file, key)
            return key
        }
        val blob = file.readBytes()
        val secret = keyStore().getKey(PLAIN_ALIAS, null) as? SecretKey
            ?: error("Secure storage key is missing; saved SSH keys cannot be decrypted. Clear app data to reset.")
        val cipher = Cipher.getInstance(TRANSFORM)
        cipher.init(Cipher.DECRYPT_MODE, secret, GCMParameterSpec(128, blob, 0, 12))
        return cipher.doFinal(blob, 12, blob.size - 12)
    }

    private fun wrapPlain(file: File, key: ByteArray) {
        val cipher = Cipher.getInstance(TRANSFORM).apply { init(Cipher.ENCRYPT_MODE, plainWrapKey()) }
        file.writeBytes(cipher.iv + cipher.doFinal(key)) // 12-byte IV, then ciphertext+tag
    }

    private fun plainWrapKey(): SecretKey =
        (keyStore().getKey(PLAIN_ALIAS, null) as? SecretKey) ?: generate(PLAIN_ALIAS, authRequired = false)

    // ---- locked mode -----------------------------------------------------------------------

    /** A cipher for BiometricPrompt that, once authenticated, can open master.locked. */
    fun unlockCipher(dir: File): Cipher {
        val blob = File(dir, LOCK_FILE).readBytes()
        return Cipher.getInstance(TRANSFORM).apply {
            init(Cipher.DECRYPT_MODE, lockKey(), GCMParameterSpec(128, blob, 0, 12))
        }
    }

    /** Call with the authenticated cipher from [unlockCipher]. */
    fun openLocked(dir: File, cipher: Cipher): ByteArray {
        val blob = File(dir, LOCK_FILE).readBytes()
        return cipher.doFinal(blob, 12, blob.size - 12)
    }

    /** A cipher for BiometricPrompt that, once authenticated, can wrap the master key. */
    fun lockCipher(): Cipher =
        Cipher.getInstance(TRANSFORM).apply { init(Cipher.ENCRYPT_MODE, lockKey()) }

    /** Turn the lock on. [cipher] is the authenticated cipher from [lockCipher]. */
    fun enableLock(dir: File, cipher: Cipher) {
        val key = plainKey(dir)
        File(dir, LOCK_FILE).writeBytes(cipher.iv + cipher.doFinal(key))
        // Only now remove the unauthenticated copy, so a crash never leaves us without a key.
        File(dir, PLAIN_FILE).delete()
        keyStore().deleteEntry(PLAIN_ALIAS)
    }

    /** Turn the lock off, given the master key recovered through [openLocked]. */
    fun disableLock(dir: File, key: ByteArray) {
        wrapPlain(File(dir, PLAIN_FILE), key)
        File(dir, LOCK_FILE).delete()
        keyStore().deleteEntry(LOCK_ALIAS)
    }

    private fun lockKey(): SecretKey =
        (keyStore().getKey(LOCK_ALIAS, null) as? SecretKey) ?: generate(LOCK_ALIAS, authRequired = true)

    private fun generate(alias: String, authRequired: Boolean): SecretKey {
        fun build(strongBox: Boolean): SecretKey {
            val spec = KeyGenParameterSpec.Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(256)
                .apply {
                    if (authRequired) {
                        setUserAuthenticationRequired(true)
                        // Authenticate for every single use (timeout 0), via CryptoObject.
                        if (Build.VERSION.SDK_INT >= 30) {
                            setUserAuthenticationParameters(
                                0,
                                KeyProperties.AUTH_BIOMETRIC_STRONG or KeyProperties.AUTH_DEVICE_CREDENTIAL
                            )
                        } else {
                            setUserAuthenticationValidityDurationSeconds(-1)
                        }
                        // Enrolling another fingerprint must not destroy the key (and every saved SSH key).
                        setInvalidatedByBiometricEnrollment(false)
                    }
                    if (strongBox && Build.VERSION.SDK_INT >= 28) setIsStrongBoxBacked(true)
                }
                .build()
            return KeyGenerator.getInstance(KeyProperties.KEY_ALGORITHM_AES, "AndroidKeyStore")
                .apply { init(spec) }.generateKey()
        }
        return try {
            build(strongBox = true)
        } catch (e: Exception) { // no StrongBox on this device
            build(strongBox = false)
        }
    }

    private const val TRANSFORM = "AES/GCM/NoPadding"
}
