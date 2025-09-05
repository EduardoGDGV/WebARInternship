import './style.css';
import Edit from './edit';
import metadata from './block.json';
import { registerBlockType } from '@wordpress/blocks';
import { __ } from '@wordpress/i18n';

registerBlockType(metadata.name, {
    apiVersion: 2,
    title: __('3D Asset Editor', 'wp-3d-asset-editor'),
    category: 'widgets',
    icon: 'media-code',
    supports: {
        html: false,
    },
    attributes: {
        sceneId: {
            type: 'number',
            default: 0,
        },
    },
    edit: Edit,
    save: () => null, // dynamic render, saved in the backend
});
