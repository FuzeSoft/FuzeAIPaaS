
export function bufferToBase64url(buf) {
  const bytes = new Uint8Array(buf)
  let str = ''
  for (let i = 0; i < bytes.length; i++) str += String.fromCharCode(bytes[i])
  return btoa(str).replace(/\+/g, '-').replace(/\
}

export function base64urlToBuffer(str) {
  let b64 = str.replace(/-/g, '+').replace(/_/g, '/')
  while (b64.length % 4) b64 += '='
  const bin = atob(b64)
  const bytes = new Uint8Array(bin.length)
  for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i)
  return bytes.buffer
}

export function parseCreationOptions(data) {
  const publicKey = { ...data }
  if (publicKey.challenge) publicKey.challenge = base64urlToBuffer(publicKey.challenge)
  if (publicKey.user && publicKey.user.id) publicKey.user.id = base64urlToBuffer(publicKey.user.id)
  
  return publicKey
}

export function parseRequestOptions(data) {
  const publicKey = { ...data }
  if (publicKey.challenge) publicKey.challenge = base64urlToBuffer(publicKey.challenge)
  if (Array.isArray(publicKey.allowCredentials)) {
    publicKey.allowCredentials = publicKey.allowCredentials.map((c) => ({
      ...c,
      id: c.id ? base64urlToBuffer(c.id) : c.id,
    }))
  }
  return publicKey
}

function credToJSON(cred) {
  if (cred instanceof ArrayBuffer || ArrayBuffer.isView(cred)) {
    return bufferToBase64url(cred)
  }
  if (cred && typeof cred === 'object') {
    const out = {}
    for (const [k, v] of Object.entries(cred)) {
      out[k] = credToJSON(v)
    }
    return out
  }
  return cred
}

export function attestationToJSON(cred) {
  return JSON.stringify({
    id: cred.id,
    rawId: bufferToBase64url(cred.rawId),
    type: cred.type,
    clientExtensionResults: cred.getClientExtensionResults(),
    response: {
      clientDataJSON: bufferToBase64url(cred.response.clientDataJSON),
      attestationObject: bufferToBase64url(cred.response.attestationObject),
      
    },
  })
}

export function assertionToJSON(cred) {
  return JSON.stringify({
    id: cred.id,
    rawId: bufferToBase64url(cred.rawId),
    type: cred.type,
    clientExtensionResults: cred.getClientExtensionResults(),
    response: {
      clientDataJSON: bufferToBase64url(cred.response.clientDataJSON),
      authenticatorData: bufferToBase64url(cred.response.authenticatorData),
      signature: bufferToBase64url(cred.response.signature),
      userHandle: cred.response.userHandle ? bufferToBase64url(cred.response.userHandle) : undefined,
    },
  })
}

export function isWebAuthnSupported() {
  return !!(window.PublicKeyCredential && navigator.credentials)
}