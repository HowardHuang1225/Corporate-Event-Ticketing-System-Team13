export async function generateTOTP(secretString: string, windowSeconds = 15): Promise<string> {
  const enc = new TextEncoder()
  const keyData = enc.encode(secretString)
  const cryptoKey = await crypto.subtle.importKey(
    "raw", keyData, { name: "HMAC", hash: "SHA-256" }, false, ["sign"]
  )
  
  // Calculate the counter
  const counter = Math.floor(Date.now() / 1000 / windowSeconds)
  const counterBuffer = new ArrayBuffer(8)
  const dataView = new DataView(counterBuffer)
  dataView.setBigUint64(0, BigInt(counter), false) // Big Endian
  
  // HMAC-SHA256
  const signature = await crypto.subtle.sign("HMAC", cryptoKey, new Uint8Array(counterBuffer))
  const hashArray = new Uint8Array(signature)
  
  // Dynamic Truncation
  const offset = hashArray[hashArray.length - 1] & 0xf
  const binary = ((hashArray[offset] & 0x7f) << 24) |
                 ((hashArray[offset + 1] & 0xff) << 16) |
                 ((hashArray[offset + 2] & 0xff) << 8) |
                 (hashArray[offset + 3] & 0xff)
                 
  const otp = binary % 1000000
  return otp.toString().padStart(6, '0')
}
