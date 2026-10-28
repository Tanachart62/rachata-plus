// Accept the user's original dimensions; upload a bounded JPEG for API memory.
export async function prepareCover(file: File): Promise<File> {
  if (!['image/jpeg', 'image/png'].includes(file.type) || !file.size) throw new Error('เลือกภาพ JPEG หรือ PNG ที่เปิดอ่านได้')
  const image = await createImageBitmap(file)
  try {
    const scale = Math.min(1, 2048 / Math.max(image.width, image.height))
    const canvas = document.createElement('canvas')
    canvas.width = Math.max(1, Math.round(image.width * scale))
    canvas.height = Math.max(1, Math.round(image.height * scale))
    const context = canvas.getContext('2d')
    if (!context) throw new Error('เตรียมภาพไม่ได้')
    context.fillStyle = '#fff'
    context.fillRect(0, 0, canvas.width, canvas.height)
    context.drawImage(image, 0, 0, canvas.width, canvas.height)
    let blob: Blob | null = null
    for (const quality of [0.85, 0.7, 0.55, 0.4]) {
      blob = await new Promise<Blob | null>(resolve => canvas.toBlob(resolve, 'image/jpeg', quality))
      if (blob && blob.size <= 2 * 1024 * 1024) break
    }
    if (!blob || blob.size > 2 * 1024 * 1024) throw new Error('เตรียมภาพไม่ได้ กรุณาลองภาพอื่น')
    return new File([blob], 'cover.jpg', { type: 'image/jpeg' })
  } finally { image.close() }
}
export async function coverBase64(file: File): Promise<string> {
  const bytes = new Uint8Array(await file.arrayBuffer())
  let text = ''
  for (let index = 0; index < bytes.length; index += 8192) text += String.fromCharCode(...bytes.subarray(index, index + 8192))
  return btoa(text)
}
