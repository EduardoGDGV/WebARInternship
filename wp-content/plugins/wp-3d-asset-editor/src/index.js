import './style.css';
import Edit from './edit';
import metadata from './block.json';
import { registerBlockType } from '@wordpress/blocks';
import { __ } from '@wordpress/i18n';

registerBlockType(metadata.name, {
    edit: Edit,
    save: () => null, // dynamic render, saved in the backend
});
