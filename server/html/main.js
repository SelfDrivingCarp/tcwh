let webhookSection = document.querySelector('section#webhooks');
let webhookEditDialog = document.querySelector('dialog#edit_webhook')
let editingWebhook = { sub: '', label: '', enabled: true };

function deleteOAuth() {
    fetch('/oauth', { method: 'DELETE' })
        .then(() => {
            window.location.reload();
            showOAuth(false);
        });
}

function showOAuth(show) {
    let [showSelect, hideSelect] = show ? ['#twitch>#details', '#twitch>#new'] : ['#twitch>#new', '#twitch>#details'];
    document.querySelector(showSelect).classList.remove('hidden');
    document.querySelector(hideSelect).classList.add('hidden');
}

function setupWebhooks() {
    let webhookAddDialog = document.querySelector('dialog#add_webhook');
    webhookSection.querySelector('button#new').addEventListener('click', () => webhookAddDialog.showModal());
    webhookAddDialog.querySelector('button#cancel').addEventListener('click', () => webhookAddDialog.close());


    webhookEditDialog.querySelector('#enable').addEventListener('click', () => {
        let action = editingWebhook.enabled ? 'Disable' : 'Enable';
        if (confirm(`${action} webhook ${editingWebhook.label}?`)) {
            fetch(`/webhooks/${editingWebhook.label}`, {
                method: 'PUT',
            }).then((_) => {
                window.location.reload();
            });
        }
    });

    webhookEditDialog.querySelector('#delete').addEventListener('click', () => {
        if (confirm(`Delete webhook ${editingWebhook.label}?`)) {
            fetch(`/webhooks/${editingWebhook.label}`, {
                method: 'DELETE',
            }).then((_) => {
                window.location.reload();
            })
        }
    });

    webhookEditDialog.querySelector('#cancel').addEventListener('click', () => webhookEditDialog.close());
}

function showWebhookEdit() {
    webhookEditDialog.querySelector('#label').innerText = editingWebhook.label;
    let enable = webhookEditDialog.querySelector('#enable');
    enable.innerText = editingWebhook.enabled ? 'Disable' : 'Enable';
    webhookEditDialog.showModal();
}

function renderWebhook(parent, wh) {
    let label = document.createElement('label');
    label.innerText = wh.label;
    parent.appendChild(label);

    let enabled = document.createElement('div');
    enabled.innerText = wh.enabled ? 'Enabled' : 'Disabled';
    parent.appendChild(enabled);

    let edit = document.createElement('button');
    edit.innerText = 'Edit';
    edit.addEventListener('click', () => {
        editingWebhook = wh;
        showWebhookEdit();
    });
    parent.appendChild(edit);
}

function updateUser() {
    fetch('/state')
        .then((resp) => resp.json())
        .then((resp) => {
            document.getElementById("user").innerText = resp.user;
            if (resp.oauth && resp.oauth.twitch_login) {
                let twitchDetails = document.querySelector('#twitch>#details')
                let twitchDetailFields = twitchDetails.querySelector('#fields');
                twitchDetailFields.querySelector('#twitch_id').innerText = resp.oauth.twitch_id;
                twitchDetailFields.querySelector('#twitch_login').innerText = resp.oauth.twitch_login;
                twitchDetailFields.querySelector('#last_validated').innerText = resp.oauth.last_validated;
                showOAuth(true);

                twitchDetails.querySelector('button#delete').addEventListener('click', () => deleteOAuth());
            } else {
                showOAuth(false);
            }

            let webhookDetails = webhookSection.querySelector('#details');
            if (resp.webhooks) {
                resp.webhooks.forEach((wh) => renderWebhook(webhookDetails, wh));
            }
        });
}

function main() {
    setupWebhooks();
    updateUser();
}

main();
