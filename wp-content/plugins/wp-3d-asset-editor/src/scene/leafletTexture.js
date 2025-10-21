import L from 'leaflet';
import leafletImage from 'leaflet-image';

export function createOffscreenMap(containerId = 'leaflet-map-offscreen', opts = {}) {
    let mapContainer = document.getElementById(containerId);
    if (!mapContainer) {
        mapContainer = document.createElement('div');
        mapContainer.id = containerId;
        mapContainer.style.width = opts.size || '1024px';
        mapContainer.style.height = opts.size || '1024px';
        mapContainer.style.position = 'absolute';
        mapContainer.style.top = '-9999px';
        document.body.appendChild(mapContainer);
    }

    const map = L.map(mapContainer, Object.assign({
        center: opts.center || [-23.55719, -46.73015],
        zoom: opts.zoom || 16,
        attributionControl: false,
        dragging: false,
        interactive: false
    }, opts));

    L.tileLayer('https://{s}.tile.openstreetmap.org/{z}/{x}/{y}.png', { crossOrigin: true }).addTo(map);

    return { map, mapContainer };
}

export function renderMapToDynamicTexture(map, dynamicTexture) {
    leafletImage(map, (err, canvas) => {
        if (err || !canvas) return console.error('leaflet-image failed', err);
        const ctx = dynamicTexture.getContext();
        if (!ctx) return console.error('dynamicTexture context missing');
        ctx.drawImage(canvas, 0, 0, dynamicTexture.getSize().width, dynamicTexture.getSize().height);
        dynamicTexture.update();
    });
}