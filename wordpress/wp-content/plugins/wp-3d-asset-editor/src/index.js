import './style.css';
import Edit from './edit';
import metadata from './block.json';
import { registerBlockType } from '@wordpress/blocks';

// Register block
registerBlockType(metadata.name, {
    edit: Edit,
    save: () => null, // dynamic block — handled by render.php
});
