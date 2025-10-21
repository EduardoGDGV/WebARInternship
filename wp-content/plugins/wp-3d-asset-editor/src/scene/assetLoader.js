import { SceneLoader, MeshBuilder, Vector3 } from '@babylonjs/core';

export async function loadAssetMesh(assetUrl, scene) {
    if (!assetUrl) return MeshBuilder.CreateBox('fallback', { size: 1 }, scene);
    try {
        const url = new URL(assetUrl, window.location.origin);
        const root = url.href.substring(0, url.href.lastIndexOf('/') + 1);
        const file = url.pathname.split('/').pop();
        const res = await SceneLoader.ImportMeshAsync('', root, file, scene);
        let mesh = res.meshes.find(m => m && typeof m.getTotalVertices === 'function') || res.meshes[0];
        if (mesh && mesh.rotationQuaternion) {
            mesh.rotation = mesh.rotationQuaternion.toEulerAngles();
            mesh.rotationQuaternion = null;
        }
        return mesh;
    } catch (e) {
        console.warn('Failed to load glTF asset:', e);
        return MeshBuilder.CreateBox('fallback', { size: 1 }, scene);
    }
}