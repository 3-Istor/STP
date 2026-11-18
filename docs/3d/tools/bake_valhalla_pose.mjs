// Freeze the displayed source pose with Three.js, preserving positions, normals and tangents.
// Usage: STP_3D_URL=http://127.0.0.1:41769 node docs/3d/tools/bake_valhalla_pose.mjs /tmp/valhalla-pose.json
// No generated pictures: browser evaluates the real glTF skinning transforms.
import { chromium } from '../../../web/node_modules/playwright-core/index.mjs'
import { writeFile } from 'node:fs/promises'
import { dirname, join } from 'node:path'

const output = process.argv[2]
if (!output) throw new Error('Output JSON path required')
const browser = await chromium.launch({ headless: true, args: ['--no-sandbox'] })
try {
  const page = await browser.newPage()
  await page.goto(process.env.STP_3D_URL ?? 'http://127.0.0.1:41769')
  const result = await page.evaluate(async () => {
    const T = await import('/node_modules/three/build/three.module.js')
    const { GLTFLoader } = await import('/node_modules/three/examples/jsm/loaders/GLTFLoader.js')
    const gltf = await new GLTFLoader().loadAsync('/models/Valhalla2025.glb')
    gltf.scene.updateMatrixWorld(true)
    const encode = array => {
      const bytes = new Uint8Array(array.buffer)
      let value = ''
      for (let i = 0; i < bytes.length; i += 32768) value += String.fromCharCode(...bytes.subarray(i, i + 32768))
      return btoa(value)
    }
    const meshes = []
    gltf.scene.traverse(mesh => {
      if (!mesh.isMesh) return
      const geometry = mesh.geometry, count = geometry.attributes.position.count
      if (mesh.isSkinnedMesh) mesh.skeleton.update()
      const position = new Float32Array(count * 3), normal = geometry.attributes.normal ? new Float32Array(count * 3) : null
      const tangent = geometry.attributes.tangent ? new Float32Array(count * 4) : null
      const value = new T.Vector3(), indices = new T.Vector4(), weights = new T.Vector4()
      const skin = new T.Matrix4()
      for (let i = 0; i < count; i++) {
        mesh.getVertexPosition(i, value).toArray(position, i * 3)
        if (mesh.isSkinnedMesh) {
          indices.fromBufferAttribute(geometry.attributes.skinIndex, i)
          weights.fromBufferAttribute(geometry.attributes.skinWeight, i)
          skin.elements.fill(0)
          for (let bone = 0; bone < 4; bone++) {
            const index = indices.getComponent(bone), weight = weights.getComponent(bone)
            for (let axis = 0; axis < 16; axis++) skin.elements[axis] += mesh.skeleton.boneMatrices[index * 16 + axis] * weight
          }
          skin.premultiply(mesh.bindMatrixInverse).multiply(mesh.bindMatrix)
        } else skin.identity()
        if (normal) value.fromBufferAttribute(geometry.attributes.normal, i).transformDirection(skin).toArray(normal, i * 3)
        if (tangent) {
          value.fromBufferAttribute(geometry.attributes.tangent, i).transformDirection(skin).toArray(tangent, i * 4)
          tangent[i * 4 + 3] = geometry.attributes.tangent.getW(i)
        }
      }
      meshes.push({ ...gltf.parser.associations.get(mesh), matrix: mesh.matrixWorld.toArray(),
        position: encode(position), normal: normal ? encode(normal) : null, tangent: tangent ? encode(tangent) : null })
    })
    const bounds = new T.Box3().setFromObject(gltf.scene)
    return { bounds: [bounds.min.toArray(), bounds.max.toArray()], meshes }
  })
  await writeFile(output, JSON.stringify(result))
  const fiat = await page.evaluate(async () => {
    const { GLTFLoader } = await import('/node_modules/three/examples/jsm/loaders/GLTFLoader.js')
    const gltf = await new GLTFLoader().loadAsync('/models/Multipla1998.glb')
    gltf.scene.updateMatrixWorld(true)
    const nodes = []
    gltf.scene.traverse(mesh => { if (mesh.isMesh) nodes.push({ mesh: true, name: mesh.name, matrix: mesh.matrixWorld.toArray() }) })
    return { nodes }
  })
  await writeFile(join(dirname(output), 'Multipla1998-measured.json'), JSON.stringify(fiat))
  process.stdout.write(`Frozen ${result.meshes.length} meshes; original GLB unchanged; bounds ${JSON.stringify(result.bounds)}\n`)
} finally { await browser.close() }
