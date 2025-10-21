import './css/style.css';
import Edit from './edit';
import metadata from './block.json';
import { registerBlockType } from '@wordpress/blocks';

registerBlockType(metadata.name, {
    edit: Edit,
    save: () => null, // dynamic render, saved in the backend
});
